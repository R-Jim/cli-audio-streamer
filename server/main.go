package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/gordonklaus/portaudio"
)

// Audio parameters
const (
	SampleRate = 48000 // Hz
	Channels   = 2     // Stereo

	FramesPerBuffer = 512                            // Number of audio frames per buffer
	PacketSize      = FramesPerBuffer * Channels * 2 // 2 bytes per int16 sample
)

func main() {
	listenPort := flag.Int("port", 8080, "Port to listen for audio stream")
	serverVolume := flag.Float64("volume", 1.0, "Server-side volume adjustment (0.0 to 1.0)")
	clientControlAddrStr := flag.String("client-control-addr", "", "Client address (IP:Port) for sending control messages (e.g., 127.0.0.1:8081)")
	flag.Parse()

	if *serverVolume < 0.0 || *serverVolume > 1.0 {
		log.Fatalf("Server volume must be between 0.0 and 1.0")
	}

	// Resolve UDP address to listen on for audio stream
	audioAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", *listenPort))
	if err != nil {
		log.Fatalf("Error resolving audio listen address: %v", err)
	}

	// Create UDP listener for audio stream
	audioConn, err := net.ListenUDP("udp", audioAddr)
	if err != nil {
		log.Fatalf("Error listening on UDP for audio: %v", err)
	}
	defer audioConn.Close()

	fmt.Printf("Server started. Listening for audio on UDP port %d with server volume %.2f\\n", *listenPort, *serverVolume)
	fmt.Println("Waiting for audio stream...")
	fmt.Println("Press Ctrl+C to stop.")

	// Handle client control if address is provided
	if *clientControlAddrStr != "" {
		clientControlAddr, err := net.ResolveUDPAddr("udp", *clientControlAddrStr)
		if err != nil {
			log.Fatalf("Error resolving client control address: %v", err)
		}
		// Create a UDP connection for sending control messages
		controlConn, err := net.DialUDP("udp", nil, clientControlAddr)
		if err != nil {
			log.Fatalf("Error creating UDP control connection: %v", err)
		}
		defer controlConn.Close()

		fmt.Printf("Ready to send client volume control to %s\\n", *clientControlAddrStr)
		fmt.Println("Enter new client volume (0.0-1.0) and press Enter:")

		// Goroutine to read volume from stdin and send to client
		go func() {
			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("> ")
				input, _ := reader.ReadString('\n')
				input = input[:len(input)-1] // Remove newline

				newVolume, err := strconv.ParseFloat(input, 64)
				if err != nil {
					fmt.Println("Invalid input. Please enter a number between 0.0 and 1.0.")
					continue
				}
				if newVolume < 0.0 || newVolume > 1.0 {
					fmt.Println("Volume must be between 0.0 and 1.0.")
					continue
				}

				// Convert float64 to byte slice
				buf := new(bytes.Buffer)
				err = binary.Write(buf, binary.LittleEndian, newVolume)
				if err != nil {
					log.Printf("Error encoding volume for sending: %v", err)
					continue
				}

				_, err = controlConn.Write(buf.Bytes())
				if err != nil {
					log.Printf("Error sending client volume control: %v", err)
				} else {
					fmt.Printf("Sent client volume: %.2f\\n", newVolume)
				}
			}
		}()
	}

	// Initialize PortAudio
	err = portaudio.Initialize()
	if err != nil {
		log.Fatalf("Error initializing PortAudio: %v", err)
	}
	defer portaudio.Terminate()

	// Create output stream
	outputBuffer := make([]int16, FramesPerBuffer*Channels) // 16-bit stereo samples
	stream, err := portaudio.OpenDefaultStream(0, Channels, SampleRate, FramesPerBuffer, outputBuffer)
	if err != nil {
		log.Fatalf("Error opening default output stream: %v", err)
	}
	defer stream.Close()

	// Create a buffered channel to hold incoming audio packets
	packetChannel := make(chan []byte, 200) // Buffer up to 200 packets

	// Goroutine to read from network and send to channel
	go func() {
		for {
			buffer := make([]byte, PacketSize)
			n, _, err := audioConn.ReadFromUDP(buffer)
			if err != nil {
				log.Printf("Error reading UDP packet: %v", err)
				continue
			}
			if n == PacketSize {
				// Non-blocking send to avoid deadlocks if the channel is full
				select {
				case packetChannel <- buffer[:n]:
				default:
					log.Println("Jitter buffer is full, dropping packet.")
				}
			} else {
				log.Printf("Received packet of unexpected size: %d bytes (expected %d)", n, PacketSize)
			}
		}
	}()

	// Dynamic Jitter Buffer Management
	const (
		MinPrebufferSize = 10  // Minimum packets to start playback
		MaxBufferSize    = 200 // Corresponds to channel capacity
		TargetBufferSize = 25  // Ideal number of packets in buffer
		HighWaterMark    = 40  // Speed up playback if buffer exceeds this
		LowWaterMark     = 15  // Slow down playback if buffer falls below this
	)

	// Pre-buffering: wait until we have a minimum number of packets
	fmt.Println("Pre-buffering audio...")
	for len(packetChannel) < MinPrebufferSize {
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Println("Pre-buffering complete. Starting playback.")

	// Start the stream
	err = stream.Start()
	if err != nil {
		log.Fatalf("Error starting output stream: %v", err)
	}
	defer stream.Stop()

	for {
		var receiveBuffer []byte

		// Dynamic adjustment based on buffer level
		bufferLevel := len(packetChannel)
		if bufferLevel > HighWaterMark {
			// Buffer is too full, consume two packets to speed up
			receiveBuffer = <-packetChannel
			// We'll process this packet and the next one in this loop iteration
		} else if bufferLevel < LowWaterMark && bufferLevel > 0 {
			// Buffer is running low, slow down by potentially adding silence
			// For simplicity, we'll just process one packet but could add silence here
			receiveBuffer = <-packetChannel
		} else {
			// Buffer is within target range
			receiveBuffer = <-packetChannel
		}

		// Read int16 samples from byte buffer
		reader := bytes.NewReader(receiveBuffer)
		samplesRead := 0
		for i := 0; i < len(outputBuffer); i++ {
			var sample int16
			err = binary.Read(reader, binary.LittleEndian, &sample)
			if err != nil {
				// This can happen if a packet is smaller than expected
				break
			}
			// Apply server-side volume adjustment
			outputBuffer[i] = int16(float64(sample) * *serverVolume)
			samplesRead++
		}

		// If we need to speed up, process a second packet
		if bufferLevel > HighWaterMark && len(packetChannel) > 0 {
			nextPacket := <-packetChannel
			reader = bytes.NewReader(nextPacket)
			for i := samplesRead; i < len(outputBuffer); i++ {
				var sample int16
				err = binary.Read(reader, binary.LittleEndian, &sample)
				if err != nil {
					break
				}
				outputBuffer[i] = int16(float64(sample) * *serverVolume)
			}
		}

		// Write audio frames to output device
		err = stream.Write()
		if err != nil {
			log.Printf("Error writing to stream: %v", err)
		}
	}
}
