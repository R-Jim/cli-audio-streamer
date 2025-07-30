package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestClientVolumeAdjustment tests the client-side volume adjustment logic.
func TestClientVolumeAdjustment(t *testing.T) {
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

// TestDecodeVolumeControlMessage tests the logic for decoding a volume control message from the server.
func TestDecodeVolumeControlMessage(t *testing.T) {
	volume := 0.75
	controlBuffer := new(bytes.Buffer)
	err := binary.Write(controlBuffer, binary.LittleEndian, volume)
	if err != nil {
		t.Fatalf("Failed to write test volume to buffer: %v", err)
	}

	var receivedVolume float64
	buf := bytes.NewReader(controlBuffer.Bytes())
	err = binary.Read(buf, binary.LittleEndian, &receivedVolume)
	if err != nil {
		t.Fatalf("Failed to decode volume from buffer: %v", err)
	}

	if receivedVolume != volume {
		t.Errorf("expected volume %.2f, got %.2f", volume, receivedVolume)
	}
}
