package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic" // For atomic.Value

	"github.com/gordonklaus/portaudio"
)

// Audio parameters
const (
	SampleRate      = 48000 // Hz
	Channels        = 2     // Stereo
	FramesPerBuffer = 512   // Number of audio frames per buffer
	ServerAudioPort = 8080  // Default server port for audio
)

// findWasapiStereoMixDevice searches for a "Stereo Mix" device on the "Windows WASAPI" host API.
func findWasapiStereoMixDevice(devices []*portaudio.DeviceInfo) (device *portaudio.DeviceInfo, found bool) {
	for _, info := range devices {
		// Case-insensitive search for "Stereo Mix" on the WASAPI host API
		if info.HostApi != nil && info.HostApi.Name == "Windows WASAPI" && info.MaxInputChannels > 0 && strings.Contains(strings.ToLower(info.Name), "stereo mix") {
			return info, true
		}
	}
	return nil, false
}

func main() {
	serverIP := flag.String("server", "127.0.0.1", "Server IP address for audio stream")
	initialVolume := flag.Float64("volume", 1.0, "Initial client-side volume adjustment (0.0 to 1.0)")
	controlPort := flag.Int("control-port", 8081, "Port to listen for server control messages")
	listDevices := flag.Bool("list-devices", false, "List available audio input devices and exit.")
	flag.Parse()

	if *initialVolume < 0.0 || *initialVolume > 1.0 {
		log.Fatalf("Initial volume must be between 0.0 and 1.0")
	}

	// Initialize PortAudio for device listing or streaming
	err := portaudio.Initialize()
	if err != nil {
		log.Fatalf("Error initializing PortAudio: %v", err)
	}
	defer portaudio.Terminate()

	if *listDevices {
		fmt.Println("Available Audio Input Devices:")
		devices, err := portaudio.Devices()
		if err != nil {
			log.Fatalf("Error listing devices: %v", err)
		}
		for i, info := range devices {
			if info.MaxInputChannels > 0 { // Only list input devices
				fmt.Printf("  [%d] %s (Host API: %s)\\n", i, info.Name, info.HostApi.Name)
			}
		}
		return // Exit after listing devices
	}

	// Use atomic.Value for thread-safe volume adjustment
	var currentClientVolume atomic.Value
	currentClientVolume.Store(*initialVolume)

	// Construct server address string
	serverAddrStr := fmt.Sprintf("%s:%d", *serverIP, ServerAudioPort)

	// Resolve server address for audio stream
	serverAddr, err := net.ResolveUDPAddr("udp", serverAddrStr)
	if err != nil {
		log.Fatalf("Error resolving server address: %v", err)
	}

	// Create UDP connection for audio stream
	audioConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		log.Fatalf("Error creating UDP audio connection: %v", err)
	}
	defer audioConn.Close()

	// Start goroutine to listen for control messages from server
	go func() {
		controlAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", *controlPort))
		if err != nil {
			log.Printf("Error resolving control listen address: %v", err)
			return
		}
		controlConn, err := net.ListenUDP("udp", controlAddr)
		if err != nil {
			log.Printf("Error listening on control UDP: %v", err)
			return
		}
		defer controlConn.Close()

		log.Printf("Client control listener started on :%d", *controlPort)

		controlBuffer := make([]byte, 8) // For float64 volume
		for {
			n, _, err := controlConn.ReadFromUDP(controlBuffer)
			if err != nil {
				log.Printf("Error reading control UDP packet: %v", err)
				continue
			}
			if n == 8 {
				var receivedVolume float64
				buf := bytes.NewReader(controlBuffer[:n])
				err = binary.Read(buf, binary.LittleEndian, &receivedVolume)
				if err != nil {
					log.Printf("Error decoding received volume: %v", err)
					continue
				}
				if receivedVolume >= 0.0 && receivedVolume <= 1.0 {
					currentClientVolume.Store(receivedVolume)
					log.Printf("Client volume updated by server to: %.2f", receivedVolume)
				} else {
					log.Printf("Received invalid volume value: %.2f", receivedVolume)
				}
			} else {
				log.Printf("Received control packet of unexpected size: %d bytes (expected 8)", n)
			}
		}
	}()

	// Buffer for sending data over UDP.
	sendBuffer := new(bytes.Buffer)

	// audioCallback is the function called by PortAudio when new audio data is available.
	audioCallback := func(in []int16) {
		sendBuffer.Reset() // Clear buffer for new data

		// Get current volume.
		vol := currentClientVolume.Load().(float64)

		// Apply volume adjustment and write to buffer.
		for _, sample := range in {
			adjustedSample := int16(float64(sample) * vol)
			err := binary.Write(sendBuffer, binary.LittleEndian, adjustedSample)
			if err != nil {
				log.Printf("Error writing sample to buffer: %v", err)
				// Continue processing the rest of the buffer.
			}
		}

		// Send the audio buffer over UDP if it has data.
		if sendBuffer.Len() > 0 {
			_, err := audioConn.Write(sendBuffer.Bytes())
			if err != nil {
				log.Printf("Error sending UDP packet: %v", err)
			}
		}
	}

	// Open audio input stream
	var stream *portaudio.Stream

	// Attempt to find and use "Stereo Mix" on the WASAPI Host API
	var stereoMixDevice *portaudio.DeviceInfo
	var stereoMixFound bool
	devices, err := portaudio.Devices()
	if err != nil {
		log.Printf("Warning: Error listing devices: %v. Will try default input.", err)
	} else {
		stereoMixDevice, stereoMixFound = findWasapiStereoMixDevice(devices)
		if stereoMixFound {
			log.Printf("Found 'Stereo Mix' device on %s: %s. Attempting to use it.", stereoMixDevice.HostApi.Name, stereoMixDevice.Name)
			param := portaudio.StreamParameters{
				Input: portaudio.StreamDeviceParameters{
					Device:   stereoMixDevice,
					Channels: Channels,
					Latency:  stereoMixDevice.DefaultLowInputLatency,
				},
				SampleRate:      SampleRate,
				FramesPerBuffer: FramesPerBuffer,
			}
			stream, err = portaudio.OpenStream(param, audioCallback)
		}
	}

	// If Stereo Mix on WASAPI is not found, or if opening it fails, fall back to the default device.
	if !stereoMixFound || err != nil {
		if stereoMixFound {
			// This executes if we found the device but failed to open it.
			log.Printf("Warning: Failed to open 'Stereo Mix' stream on WASAPI: %v. Falling back to default input device.", err)
		} else {
			// This executes if we never found the device.
			log.Println("Warning: 'Stereo Mix' on WASAPI not found. Falling back to default input device.")
		}

		// Open the default stream as a fallback.
		stream, err = portaudio.OpenDefaultStream(Channels, 0, SampleRate, FramesPerBuffer, audioCallback)
		if err != nil {
			// If even the default stream fails, we have to exit.
			log.Fatalf("Error opening default input stream after fallback: %v", err)
		}
	}
	defer stream.Close()

	if stereoMixFound {
		fmt.Println("Using 'Stereo Mix' on WASAPI for audio input.")
	} else {
		fmt.Println("Using default input device.")
	}

	// Start the stream
	err = stream.Start()
	if err != nil {
		log.Fatalf("Error starting stream: %v", err)
	}
	defer stream.Stop()

	fmt.Println("Streaming... Press Ctrl+C to stop.")

	// Block the main goroutine indefinitely
	select {}
}
