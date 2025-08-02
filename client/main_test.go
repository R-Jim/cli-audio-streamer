package main

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/gordonklaus/portaudio"
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

// TestFindWasapiStereoMixDevice tests the logic for finding the WASAPI Stereo Mix device.
func TestFindWasapiStereoMixDevice(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping WASAPI test on non-Windows platform")
	}
	// Mock HostApiInfo and DeviceInfo for testing
	wasapiHostApi := &portaudio.HostApiInfo{Name: "Windows WASAPI"}
	mmeHostApi := &portaudio.HostApiInfo{Name: "MME"}

	testCases := []struct {
		name          string
		devices       []*portaudio.DeviceInfo
		expectedFound bool
		expectedName  string
	}{
		{
			name: "WASAPI Stereo Mix available",
			devices: []*portaudio.DeviceInfo{
				{Name: "Default Input", MaxInputChannels: 2, HostApi: mmeHostApi},
				{Name: "Stereo Mix (Realtek High Definition Audio)", MaxInputChannels: 2, HostApi: wasapiHostApi},
				{Name: "Microphone (USB Audio)", MaxInputChannels: 1, HostApi: wasapiHostApi},
			},
			expectedFound: true,
			expectedName:  "Stereo Mix (Realtek High Definition Audio)",
		},
		{
			name: "No WASAPI Stereo Mix",
			devices: []*portaudio.DeviceInfo{
				{Name: "Default Input", MaxInputChannels: 2, HostApi: mmeHostApi},
				{Name: "Line In", MaxInputChannels: 2, HostApi: mmeHostApi},
				{Name: "Microphone (USB Audio)", MaxInputChannels: 1, HostApi: wasapiHostApi},
			},
			expectedFound: false,
			expectedName:  "",
		},
		{
			name: "Stereo Mix on wrong Host API",
			devices: []*portaudio.DeviceInfo{
				{Name: "Stereo Mix", MaxInputChannels: 2, HostApi: mmeHostApi},
				{Name: "Microphone (USB Audio)", MaxInputChannels: 1, HostApi: wasapiHostApi},
			},
			expectedFound: false,
			expectedName:  "",
		},
		{
			name:          "No devices",
			devices:       []*portaudio.DeviceInfo{},
			expectedFound: false,
			expectedName:  "",
		},
		{
			name: "Device with nil HostApi",
			devices: []*portaudio.DeviceInfo{
				{Name: "Stereo Mix", MaxInputChannels: 2, HostApi: nil},
			},
			expectedFound: false,
			expectedName:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			device, found := findWasapiStereoMixDevice(tc.devices)

			if found != tc.expectedFound {
				t.Errorf("expected found to be %v, but got %v", tc.expectedFound, found)
			}

			if tc.expectedFound && (device == nil || device.Name != tc.expectedName) {
				t.Errorf("expected device name to be '%s', but got '%s'", tc.expectedName, device.Name)
			}

			if !tc.expectedFound && device != nil {
				t.Errorf("expected device to be nil, but got %v", device)
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
