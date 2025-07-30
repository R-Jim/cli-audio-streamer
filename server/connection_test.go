package main

import (
	"net"
	"sync"
	"testing"
	"time"
)

// TestServerClientConnection simulates a basic server-client UDP connection.
// It verifies that a packet sent by a client is received by a server.
func TestServerClientConnection(t *testing.T) {
	// 1. Setup a wait group and a channel to coordinate the test.
	var wg sync.WaitGroup
	dataReceived := make(chan []byte, 1)

	// 2. Start a mock server in a goroutine.
	// This server will listen for a single UDP packet.
	serverAddr := "127.0.0.1:0" // Use port 0 to get a random free port.
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to resolve server address: %v", err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}
	defer conn.Close()

	// Get the actual address the server is listening on.
	listeningAddr := conn.LocalAddr().String()

	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer := make([]byte, 1024)
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			// Don't fail the test here, as a timeout is expected if the client fails.
			t.Logf("Server read error: %v", err)
			return
		}
		dataReceived <- buffer[:n]
	}()

	// 3. Start a mock client.
	// This client will send a single UDP packet to the server.
	clientConn, err := net.Dial("udp", listeningAddr)
	if err != nil {
		t.Fatalf("Failed to connect to mock server: %v", err)
	}
	defer clientConn.Close()

	// 4. Send a test packet from the client.
	testMessage := []byte("hello, world")
	_, err = clientConn.Write(testMessage)
	if err != nil {
		t.Fatalf("Client failed to send message: %v", err)
	}

	// 5. Wait for the server to receive the packet or timeout.
	select {
	case received := <-dataReceived:
		if string(received) != string(testMessage) {
			t.Errorf("Mismatched data. Expected '%s', got '%s'", string(testMessage), string(received))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out. Server did not receive the packet.")
	}

	// 6. Wait for the server goroutine to finish.
	wg.Wait()
}
