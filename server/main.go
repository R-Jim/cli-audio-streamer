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
	"sync/atomic"
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

// SequencedPacket represents a packet with sequence number for reordering
type SequencedPacket struct {
	sequence uint32
	data     []byte
}

// PacketReorderBuffer handles out-of-order packet reordering
type PacketReorderBuffer struct {
	buffer     map[uint32]*SequencedPacket
	nextSeq    uint32
	maxLatency int // Maximum number of packets to wait for reordering
}

// NewPacketReorderBuffer creates a new packet reordering buffer
func NewPacketReorderBuffer(maxLatency int) *PacketReorderBuffer {
	return &PacketReorderBuffer{
		buffer:     make(map[uint32]*SequencedPacket),
		nextSeq:    0,
		maxLatency: maxLatency,
	}
}

// AddPacket adds a packet with sequence number
func (prb *PacketReorderBuffer) AddPacket(seq uint32, data []byte) {
	prb.buffer[seq] = &SequencedPacket{sequence: seq, data: data}
}

// GetNextPacket returns the next packet in sequence, or nil if not available
func (prb *PacketReorderBuffer) GetNextPacket() []byte {
	if packet, exists := prb.buffer[prb.nextSeq]; exists {
		delete(prb.buffer, prb.nextSeq)
		prb.nextSeq++
		return packet.data
	}
	return nil
}

// HasPendingPackets returns true if there are packets waiting for reordering
func (prb *PacketReorderBuffer) HasPendingPackets() bool {
	return len(prb.buffer) > 0
}

// CleanupOldPackets removes packets that are too old to wait for
func (prb *PacketReorderBuffer) CleanupOldPackets() {
	for seq := range prb.buffer {
		if seq < prb.nextSeq {
			delete(prb.buffer, seq)
		}
	}
}

// JitterBuffer manages audio packets with adaptive sizing and underflow prevention
type JitterBuffer struct {
	packets       chan []byte
	bufferLevel   int64
	minBufferSize int
	maxBufferSize int
	targetSize    int
	highWaterMark int
	lowWaterMark  int
	stats         BufferStats
	reorderBuffer *PacketReorderBuffer
}

// BufferStats tracks buffer performance metrics
type BufferStats struct {
	underflows     int64
	overflows      int64
	silencePackets int64
	totalPackets   int64
}

// NewJitterBuffer creates a new adaptive jitter buffer
func NewJitterBuffer() *JitterBuffer {
	return &JitterBuffer{
		packets:       make(chan []byte, 200), // Increased capacity
		minBufferSize: 5,
		maxBufferSize: 200,
		targetSize:    20,
		highWaterMark: 30,
		lowWaterMark:  10,
		stats:         BufferStats{},
		reorderBuffer: NewPacketReorderBuffer(50), // Wait up to 50 packets for reordering
	}
}

// AddPacket adds a packet to the buffer with overflow protection
func (jb *JitterBuffer) AddPacket(packet []byte) {
	select {
	case jb.packets <- packet:
		atomic.AddInt64(&jb.bufferLevel, 1)
		atomic.AddInt64(&jb.stats.totalPackets, 1)
	default:
		atomic.AddInt64(&jb.stats.overflows, 1)
		log.Println("Jitter buffer overflow - dropping packet")
	}
}

// GetPacket retrieves a packet from the buffer
func (jb *JitterBuffer) GetPacket() ([]byte, bool) {
	select {
	case packet := <-jb.packets:
		atomic.AddInt64(&jb.bufferLevel, -1)
		return packet, true
	default:
		atomic.AddInt64(&jb.stats.underflows, 1)
		return nil, false
	}
}

// GetBufferLevel returns current buffer level
func (jb *JitterBuffer) GetBufferLevel() int {
	return int(atomic.LoadInt64(&jb.bufferLevel))
}

// ShouldInsertSilence determines if silence should be inserted
func (jb *JitterBuffer) ShouldInsertSilence() bool {
	level := jb.GetBufferLevel()
	return level < jb.lowWaterMark
}

// IsBufferFull checks if buffer is approaching capacity
func (jb *JitterBuffer) IsBufferFull() bool {
	level := jb.GetBufferLevel()
	return level > jb.highWaterMark
}

// GetStats returns current buffer statistics
func (jb *JitterBuffer) GetStats() BufferStats {
	return BufferStats{
		underflows:     atomic.LoadInt64(&jb.stats.underflows),
		overflows:      atomic.LoadInt64(&jb.stats.overflows),
		silencePackets: atomic.LoadInt64(&jb.stats.silencePackets),
		totalPackets:   atomic.LoadInt64(&jb.stats.totalPackets),
	}
}

// InsertSilencePacket creates a silent audio packet
func (jb *JitterBuffer) InsertSilencePacket() []byte {
	atomic.AddInt64(&jb.stats.silencePackets, 1)
	return make([]byte, PacketSize) // Zero-filled buffer = silence
}

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

	// Create adaptive jitter buffer
	jitterBuffer := NewJitterBuffer()

	// Goroutine to read from network and send to jitter buffer
	go func() {
		for {
			buffer := make([]byte, PacketSize+4) // +4 for sequence number
			n, _, err := audioConn.ReadFromUDP(buffer)
			if err != nil {
				log.Printf("Error reading UDP packet: %v", err)
				continue
			}
			if n == PacketSize+4 {
				// Extract sequence number (first 4 bytes)
				seq := binary.LittleEndian.Uint32(buffer[:4])
				audioData := buffer[4:n]

				// Add to reorder buffer
				jitterBuffer.reorderBuffer.AddPacket(seq, audioData)

				// Try to get packets in order and add to jitter buffer
				for {
					if orderedPacket := jitterBuffer.reorderBuffer.GetNextPacket(); orderedPacket != nil {
						jitterBuffer.AddPacket(orderedPacket)
					} else {
						break
					}
				}

				// Periodically clean up old packets
				jitterBuffer.reorderBuffer.CleanupOldPackets()
			} else if n == PacketSize {
				// Fallback for packets without sequence numbers (legacy support)
				jitterBuffer.AddPacket(buffer[:n])
			} else {
				log.Printf("Received packet of unexpected size: %d bytes (expected %d or %d)", n, PacketSize, PacketSize+4)
			}
		}
	}()

	// Goroutine to periodically log buffer statistics
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			stats := jitterBuffer.GetStats()
			level := jitterBuffer.GetBufferLevel()
			if stats.underflows > 0 || stats.overflows > 0 || stats.silencePackets > 0 {
				log.Printf("Buffer stats - Level: %d, Underflows: %d, Overflows: %d, Silence: %d, Total: %d",
					level, stats.underflows, stats.overflows, stats.silencePackets, stats.totalPackets)
			}
		}
	}()

	// Pre-buffering: wait until we have a minimum number of packets
	fmt.Println("Pre-buffering audio...")
	for jitterBuffer.GetBufferLevel() < jitterBuffer.minBufferSize {
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
		var ok bool

		// Get packet from jitter buffer or insert silence if underflow
		if jitterBuffer.ShouldInsertSilence() {
			receiveBuffer = jitterBuffer.InsertSilencePacket()
		} else {
			receiveBuffer, ok = jitterBuffer.GetPacket()
			if !ok {
				// This shouldn't happen due to ShouldInsertSilence check, but just in case
				receiveBuffer = jitterBuffer.InsertSilencePacket()
			}
		}

		// Read int16 samples from byte buffer
		reader := bytes.NewReader(receiveBuffer)
		for i := 0; i < len(outputBuffer); i++ {
			var sample int16
			err = binary.Read(reader, binary.LittleEndian, &sample)
			if err != nil {
				// This can happen if a packet is smaller than expected
				break
			}
			// Apply server-side volume adjustment
			outputBuffer[i] = int16(float64(sample) * *serverVolume)
		}

		// If buffer is too full, consume an extra packet to speed up playback
		if jitterBuffer.IsBufferFull() {
			if extraPacket, ok := jitterBuffer.GetPacket(); ok {
				// We consumed an extra packet but don't use it for audio
				// This helps reduce latency when buffer is building up
				_ = extraPacket
			}
		}

		// Write audio frames to output device
		err = stream.Write()
		if err != nil {
			log.Printf("Error writing to stream: %v", err)
		}
	}
}
