package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/go-mp3"
)

func sendPacket(conn *net.UDPConn, message []byte) error {
	_, err := conn.Write(message)
	return err
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <host:port>")
		return
	}
	serverAddrStr := os.Args[1]

	serverAddr, err := net.ResolveUDPAddr("udp", serverAddrStr)
	if err != nil {
		fmt.Println("Error resolving UDP address:", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Println("Error dialing UDP:", err)
		return
	}
	defer conn.Close()

	// Read the audio file
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("Error getting executable path:", err)
		return
	}
	file, err := os.Open(filepath.Join(filepath.Dir(exePath), "hello.mp3"))
	if err != nil {
		fmt.Println("Error opening audio file:", err)
		return
	}
	defer file.Close()

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		fmt.Println("Error creating MP3 decoder:", err)
		return
	}

	audioData, err := ioutil.ReadAll(decoder)
	if err != nil {
		fmt.Println("Error decoding MP3 file:", err)
		return
	}

	fmt.Println("Mock client started. Streaming to", serverAddr)

	// Simulate sending audio data
	const chunkSize = 2048
	for i := 0; i < len(audioData); i += chunkSize {
		end := i + chunkSize
		if end > len(audioData) {
			end = len(audioData)
		}
		chunk := audioData[i:end]

		// Pad the last chunk if it's smaller than chunkSize
		if len(chunk) < chunkSize {
			paddedChunk := make([]byte, chunkSize)
			copy(paddedChunk, chunk)
			chunk = paddedChunk
		}

		if err := sendPacket(conn, chunk); err != nil {
			fmt.Println("Error sending message:", err)
			return
		}
		fmt.Printf("Sent %d bytes\n", len(chunk))
	}
	fmt.Println("Finished sending audio file.")
}
