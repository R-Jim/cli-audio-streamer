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
	deviceName := flag.String("device-name", "", "Name of the audio input device to use.")
	deviceIndex := flag.Int("device-index", -1, "Index of the audio input device to use.")
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

	// --- Device Selection Logic ---
	var chosenDevice *portaudio.DeviceInfo
	devices, err := portaudio.Devices()
	if err != nil {
		log.Fatalf("Error listing devices for stream setup: %v", err)
	}

	if *deviceIndex >= 0 {
		// User specified a device index
		if *deviceIndex < len(devices) {
			if devices[*deviceIndex].MaxInputChannels > 0 {
				chosenDevice = devices[*deviceIndex]
				log.Printf("Using specified device by index: [%d] %s", *deviceIndex, chosenDevice.Name)
			} else {
				log.Fatalf("Device at index %d is not an input device.", *deviceIndex)
			}
		} else {
			log.Fatalf("Invalid device index: %d. Max index is %d.", *deviceIndex, len(devices)-1)
		}
	} else if *deviceName != "" {
		// User specified a device name
		var found bool
		for _, device := range devices {
			if strings.EqualFold(device.Name, *deviceName) && device.MaxInputChannels > 0 {
				chosenDevice = device
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("Specified device '%s' not found or is not an input device.", *deviceName)
		}
		log.Printf("Using specified device by name: %s", chosenDevice.Name)
	} else {
		// Default behavior: search for "Stereo Mix"
		var found bool
		chosenDevice, found = findWasapiStereoMixDevice(devices)
		if !found {
			log.Println("Warning: 'Stereo Mix' on WASAPI not found. Will fall back to default device.")
		}
	}
	// --- End of Device Selection ---

	var stream *portaudio.Stream
	var useDefault bool

	if chosenDevice != nil {
		// A specific device was chosen (by index, name, or 'Stereo Mix' search)
		log.Printf("Attempting to open stream with: %s", chosenDevice.Name)
		param := portaudio.StreamParameters{
			Input: portaudio.StreamDeviceParameters{
				Device:   chosenDevice,
				Channels: Channels,
				Latency:  chosenDevice.DefaultLowInputLatency,
			},
			SampleRate:      SampleRate,
			FramesPerBuffer: FramesPerBuffer,
		}
		stream, err = portaudio.OpenStream(param, audioCallback)
		if err != nil {
			log.Printf("Warning: Failed to open '%s': %v. Falling back to default device.", chosenDevice.Name, err)
			useDefault = true // Mark to fallback
		} else {
			fmt.Printf("Using audio input: %s\\n", chosenDevice.Name)
		}
	} else {
		// No specific device was found (e.g., 'Stereo Mix' not present)
		useDefault = true
	}

	// If a specific device failed or was never found, use the default.
	if useDefault {
		log.Println("Attempting to open stream with default input device.")
		stream, err = portaudio.OpenDefaultStream(Channels, 0, SampleRate, FramesPerBuffer, audioCallback)
		if err != nil {
			log.Fatalf("Error opening default input stream: %v", err)
		}
		defaultDevice, _ := portaudio.DefaultInputDevice()
		fmt.Printf("Using default audio input: %s\\n", defaultDevice.Name)
	}
	defer stream.Close()

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
