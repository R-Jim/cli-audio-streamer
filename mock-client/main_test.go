package main

import (
	"net"
	"testing"
)

func TestSendPacket(t *testing.T) {
	// Create a local UDP server to listen for the packet
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer serverConn.Close()

	// Create a client connection
	clientConn, err := net.DialUDP("udp", nil, serverConn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	// Send a packet
	message := []byte("test message")
	err = sendPacket(clientConn, message)
	if err != nil {
		t.Fatalf("sendPacket failed: %v", err)
	}

	// Read from the server to verify the packet was received
	buffer := make([]byte, 1024)
	n, _, err := serverConn.ReadFromUDP(buffer)
	if err != nil {
		t.Fatalf("ReadFromUDP failed: %v", err)
	}

	if string(buffer[:n]) != string(message) {
		t.Errorf("Received message %q, want %q", string(buffer[:n]), string(message))
	}
}
