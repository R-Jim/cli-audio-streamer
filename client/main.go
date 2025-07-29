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
	"time"

	"github.com/gordonklaus/portaudio"
)

// Audio parameters
const (
	SampleRate = 48000 // Hz
	Channels   = 2     // Stereo

	FramesPerBuffer = 512 // Number of audio frames per buffer
)

func main() {
	serverAddrStr := flag.String("server", "127.0.0.1:8080", "Server address (IP:Port) for audio stream")
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

	// Resolve server address for audio stream
	serverAddr, err := net.ResolveUDPAddr("udp", *serverAddrStr)
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

	// Open audio input stream
	var stream *portaudio.Stream
	inputBuffer := make([]int16, FramesPerBuffer*Channels) // 16-bit stereo samples

	// Attempt to find and use "Stereo Mix"
	stereoMixFound := false
	devices, err := portaudio.Devices()
	if err != nil {
		log.Printf("Warning: Error listing devices: %v. Will try default input.", err)
	} else {
		var stereoMixDevice *portaudio.DeviceInfo
		for _, info := range devices {
			// Case-insensitive search for "Stereo Mix" or similar
			if info.MaxInputChannels > 0 && strings.Contains(strings.ToLower(info.Name), "stereo mix") {
				stereoMixDevice = info
				stereoMixFound = true
				break
			}
		}

		if stereoMixFound {
			log.Printf("Found 'Stereo Mix' device: %s. Attempting to use it.", stereoMixDevice.Name)
			param := portaudio.StreamParameters{
				Input: portaudio.StreamDeviceParameters{
					Device:   stereoMixDevice,
					Channels: Channels,
					Latency:  stereoMixDevice.DefaultLowInputLatency,
				},
				SampleRate:      SampleRate,
				FramesPerBuffer: FramesPerBuffer,
			}
			stream, err = portaudio.OpenStream(param, inputBuffer)
		}
	}

	// If Stereo Mix not found or failed to open, fall back to default
	if !stereoMixFound || err != nil {
		if !stereoMixFound {
			log.Println("Warning: 'Stereo Mix' device not found. Falling back to default input device.")
		} else {
			log.Printf("Warning: Failed to open 'Stereo Mix' stream: %v. Falling back to default input device.", err)
		}
		stream, err = portaudio.OpenDefaultStream(Channels, 0, SampleRate, FramesPerBuffer, inputBuffer)
	}

	if err != nil {
		log.Fatalf("Error opening input stream (after trying Stereo Mix and default): %v", err)
	}
	defer stream.Close()

	// Start the stream
	err = stream.Start()
	if err != nil {
		log.Fatalf("Error starting input stream: %v", err)
	}
	defer stream.Stop()

	fmt.Printf("Client started. Streaming audio to %s\\n", *serverAddrStr)
	fmt.Printf("Initial client-side volume: %.2f. Listening for server volume control on port %d\\n", currentClientVolume.Load().(float64), *controlPort)
	if stereoMixFound {
		fmt.Println("Attempting to use 'Stereo Mix' for audio input.")
	} else {
		fmt.Println("Using default input device (could not find 'Stereo Mix').")
	}
	fmt.Println("Press Ctrl+C to stop.")

	// Buffer for sending data
	sendBuffer := new(bytes.Buffer)

	for {
		// Read audio frames from input device
		err = stream.Read()
		if err != nil {
			log.Printf("Error reading from stream: %v", err)
			continue
		}

		sendBuffer.Reset() // Clear buffer for new data

		// Get current volume
		vol := currentClientVolume.Load().(float64)

		// Apply volume adjustment and write to buffer
		for i := 0; i < len(inputBuffer); i++ {
			// Apply volume: float64(sample) * volume
			adjustedSample := int16(float64(inputBuffer[i]) * vol)
			err = binary.Write(sendBuffer, binary.LittleEndian, adjustedSample)
			if err != nil {
				log.Printf("Error writing sample to buffer: %v", err)
				continue
			}
		}

		// Send the audio buffer over UDP
		_, err = audioConn.Write(sendBuffer.Bytes())
		if err != nil {
			log.Printf("Error sending UDP packet: %v", err)
			// Small delay to prevent busy-looping on network errors
			time.Sleep(10 * time.Millisecond)
		}
	}
}
