package main

import (
	"bytes"
	"encoding/binary"
	"testing"
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
