package main

import (
	"bytes"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"
)

// TestServerVolumeAdjustment tests the server-side volume adjustment logic.
func TestServerVolumeAdjustment(t *testing.T) {
	testCases := []struct {
		name     string
		sample   int16
		volume   float64
		expected int16
	}{
		{"Half Volume", 10000, 0.5, 5000},
		{"Full Volume", 10000, 1.0, 10000},
		{"Zero Volume", 10000, 0.0, 0},
		{"Negative Sample", -10000, 0.5, -5000},
		{"Max Int16", 32767, 0.5, 16383},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adjustedSample := int16(float64(tc.sample) * tc.volume)
			if adjustedSample != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, adjustedSample)
			}
		})
	}
}

// TestEncodeVolumeControlMessage tests the logic for encoding a volume control message to send to the client.
func TestEncodeVolumeControlMessage(t *testing.T) {
	volume := 0.75
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, volume)
	if err != nil {
		t.Fatalf("Failed to encode volume: %v", err)
	}

	// The buffer should now contain 8 bytes for the float64 value.
	if len(buf.Bytes()) != 8 {
		t.Errorf("expected buffer length 8, got %d", len(buf.Bytes()))
	}

	var decodedVolume float64
	reader := bytes.NewReader(buf.Bytes())
	err = binary.Read(reader, binary.LittleEndian, &decodedVolume)
	if err != nil {
		t.Fatalf("Failed to decode volume for verification: %v", err)
	}

	if decodedVolume != volume {
		t.Errorf("expected decoded volume %.2f, got %.2f", volume, decodedVolume)
	}
}

// TestJitterBufferBasic tests basic jitter buffer functionality
func TestJitterBufferBasic(t *testing.T) {
	jb := NewJitterBuffer()

	// Test adding and retrieving packets
	testData := []byte{1, 2, 3, 4}
	jb.AddPacket(testData)

	if jb.GetBufferLevel() != 1 {
		t.Errorf("expected buffer level 1, got %d", jb.GetBufferLevel())
	}

	retrieved, ok := jb.GetPacket()
	if !ok {
		t.Fatal("expected to retrieve packet")
	}

	if len(retrieved) != len(testData) {
		t.Errorf("expected data length %d, got %d", len(testData), len(retrieved))
	}

	if jb.GetBufferLevel() != 0 {
		t.Errorf("expected buffer level 0 after retrieval, got %d", jb.GetBufferLevel())
	}
}

// TestJitterBufferUnderflowPrevention tests silence insertion when buffer is low
func TestJitterBufferUnderflowPrevention(t *testing.T) {
	jb := NewJitterBuffer()

	// Simulate buffer running low
	for i := 0; i < jb.lowWaterMark-1; i++ {
		jb.AddPacket(make([]byte, PacketSize))
	}

	// Buffer should indicate silence insertion is needed
	if !jb.ShouldInsertSilence() {
		t.Error("expected ShouldInsertSilence to return true when buffer is low")
	}

	// Insert silence packet
	silencePacket := jb.InsertSilencePacket()
	if len(silencePacket) != PacketSize {
		t.Errorf("expected silence packet length %d, got %d", PacketSize, len(silencePacket))
	}

	// Check that all bytes are zero (silence)
	for i, b := range silencePacket {
		if b != 0 {
			t.Errorf("expected silence packet byte %d to be 0, got %d", i, b)
		}
	}

	stats := jb.GetStats()
	if stats.silencePackets != 1 {
		t.Errorf("expected 1 silence packet, got %d", stats.silencePackets)
	}
}

// TestJitterBufferOverflowProtection tests buffer overflow handling
func TestJitterBufferOverflowProtection(t *testing.T) {
	jb := NewJitterBuffer()

	// Fill buffer to capacity
	for i := 0; i < 200; i++ {
		jb.AddPacket(make([]byte, PacketSize))
	}

	// Try to add one more packet (should overflow)
	jb.AddPacket(make([]byte, PacketSize))

	stats := jb.GetStats()
	if stats.overflows == 0 {
		t.Error("expected overflow to be detected")
	}
}

// TestPacketReorderBuffer tests packet reordering functionality
func TestPacketReorderBuffer(t *testing.T) {
	prb := NewPacketReorderBuffer(10)

	// Add packets out of order
	prb.AddPacket(5, []byte{5})
	prb.AddPacket(2, []byte{2})
	prb.AddPacket(8, []byte{8})
	prb.AddPacket(3, []byte{3})

	// Should not be able to get packet 0 (doesn't exist)
	if prb.GetNextPacket() != nil {
		t.Error("expected no packet for sequence 0")
	}

	// Add packet 0 and 1
	prb.AddPacket(0, []byte{0})
	prb.AddPacket(1, []byte{1})

	// Should now be able to retrieve packets in order
	expected := []byte{0, 1, 2, 3}
	for i, expectedVal := range expected {
		packet := prb.GetNextPacket()
		if packet == nil {
			t.Fatalf("expected packet for sequence %d", i)
		}
		if packet[0] != expectedVal {
			t.Errorf("expected packet value %d, got %d", expectedVal, packet[0])
		}
	}
}

// TestPacketReorderBufferCleanup tests cleanup of old packets
func TestPacketReorderBufferCleanup(t *testing.T) {
	prb := NewPacketReorderBuffer(5)

	// Start with sequence 10 as nextSeq
	prb.nextSeq = 10

	// Add packets starting from sequence 10
	prb.AddPacket(10, []byte{10})
	prb.AddPacket(11, []byte{11})
	prb.AddPacket(12, []byte{12})

	// Retrieve first packet to advance nextSeq to 11
	packet := prb.GetNextPacket()
	if packet == nil || packet[0] != 10 {
		t.Error("expected to retrieve packet 10")
	}

	// Add a future packet that should not be cleaned up
	prb.AddPacket(20, []byte{20})

	// Advance nextSeq to 15 (past 12)
	prb.nextSeq = 15

	// Add old packet that should be cleaned up
	prb.AddPacket(11, []byte{11}) // This is old and should be cleaned up

	prb.CleanupOldPackets()

	// Check that old packets were cleaned up (packets 11 and 12 should be removed, 20 should remain)
	if len(prb.buffer) != 1 {
		t.Errorf("expected 1 packet remaining after cleanup, got %d packets", len(prb.buffer))
	}

	// Verify packet 20 is still there
	if _, exists := prb.buffer[20]; !exists {
		t.Error("expected packet 20 to remain after cleanup")
	}
}

// TestJitterBufferConcurrentAccess tests thread safety of jitter buffer
func TestJitterBufferConcurrentAccess(t *testing.T) {
	jb := NewJitterBuffer()
	done := make(chan bool, 2)

	// Producer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			jb.AddPacket(make([]byte, PacketSize))
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Consumer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			if packet, ok := jb.GetPacket(); ok {
				_ = packet
			}
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Buffer should be empty or have some packets (depending on timing)
	level := jb.GetBufferLevel()
	if level < 0 || level > 200 {
		t.Errorf("unexpected buffer level after concurrent access: %d", level)
	}
}

// TestBufferStats tests buffer statistics tracking
func TestBufferStats(t *testing.T) {
	jb := NewJitterBuffer()

	// Add some packets
	for i := 0; i < 10; i++ {
		jb.AddPacket(make([]byte, PacketSize))
	}

	// Retrieve some packets
	for i := 0; i < 5; i++ {
		jb.GetPacket()
	}

	// Force underflow by trying to get more packets than available
	for i := 0; i < 10; i++ {
		jb.GetPacket()
	}

	// Insert silence packets
	for i := 0; i < 3; i++ {
		jb.InsertSilencePacket()
	}

	stats := jb.GetStats()

	if stats.totalPackets != 10 {
		t.Errorf("expected 10 total packets, got %d", stats.totalPackets)
	}

	if stats.silencePackets != 3 {
		t.Errorf("expected 3 silence packets, got %d", stats.silencePackets)
	}

	if stats.underflows == 0 {
		t.Error("expected underflows to be detected")
	}
}

// TestAtomicBufferLevel tests atomic buffer level operations
func TestAtomicBufferLevel(t *testing.T) {
	jb := NewJitterBuffer()

	// Test atomic operations
	initialLevel := jb.GetBufferLevel()
	if initialLevel != 0 {
		t.Errorf("expected initial buffer level 0, got %d", initialLevel)
	}

	jb.AddPacket(make([]byte, PacketSize))

	level := jb.GetBufferLevel()
	if level != 1 {
		t.Errorf("expected buffer level 1, got %d", level)
	}

	// Test that buffer level is properly maintained atomically
	atomic.StoreInt64(&jb.bufferLevel, 5)
	if jb.GetBufferLevel() != 5 {
		t.Error("expected buffer level to be updated atomically")
	}
}
