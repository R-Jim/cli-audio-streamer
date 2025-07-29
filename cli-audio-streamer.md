# **Go CLI Audio Streamer: Client-Server Documentation**

This document outlines the creation of two Go command-line interface (CLI) applications: an audio client and an audio server. The client captures audio, streams it over UDP, and the server receives and plays it.

## **Features**

* **Lossless Audio:** Uses raw PCM (Pulse Code Modulation) for uncompressed, lossless audio transmission.  
* **L/R Audio (Stereo):** Supports two audio channels for stereo sound.  
* **Sound Adjustment:** Allows volume control on both the client (before sending) and the server (before playback).  
* **Server-Controlled Client Volume:** The server can dynamically adjust the client's outgoing audio volume.  
* **UDP Streaming:** Utilizes UDP for low-latency, connectionless audio transmission.

## **Core Concepts**

### **Audio Format**

For simplicity and lossless quality, we will use raw PCM audio with the following specifications:

* **Sample Rate:** 48000 Hz (48 kHz)  
* **Bit Depth:** 16-bit signed integers  
* **Channels:** 2 (Stereo)  
* **Interleaving:** Samples are interleaved (Left, Right, Left, Right...)

### **Audio Library: PortAudio**

We will use PortAudio for cross-platform audio input and output. PortAudio is a portable audio I/O library that provides a common API for real-time audio applications.

To use PortAudio in Go, you'll typically need a Go binding. A common one is gopacket/portaudio.

**Prerequisites for PortAudio:**

Before compiling, you need to install the PortAudio development libraries on your system:

* **Linux (Debian/Ubuntu):**  
  sudo apt-get update  
  sudo apt-get install libportaudio2 libportaudiocpp0 portaudio19-dev

* **macOS (Homebrew):**  
  brew install portaudio

* **Windows:** This is more involved. You typically need to download the PortAudio SDK, compile it, and set up your Go environment to link against it. It's often easier to use MSYS2/MinGW-w64 or a pre-compiled binary.

### **UDP Streaming**

UDP (User Datagram Protocol) is chosen for its low latency, which is crucial for real-time audio. It's connectionless, meaning it doesn't establish a persistent connection, reducing overhead. However, it offers no guarantees of delivery, order, or duplicate protection. For simple audio streaming, this is often acceptable, and lost packets might result in brief audio glitches.

## **Client Application: audio-client**

The client application will capture audio, apply volume adjustments (both initial and server-controlled), and send the audio frames over UDP to the server. It also listens on a separate port for server-initiated volume control messages.

**Windows-Specific Desktop Audio Capture:**

The client is now configured to specifically attempt to capture audio from a device named "Stereo Mix". If "Stereo Mix" is not found, it will fall back to the system's default input device.

**Steps to Prepare for Desktop Audio Capture on Windows:**

1. **Enable "Stereo Mix" (if available):**  
   * Right-click the speaker icon in your system tray and select "Sounds" or "Sound settings."  
   * Go to the "Recording" tab.  
   * Right-click in the empty space and ensure "Show Disabled Devices" and "Show Disconnected Devices" are checked.  
   * If "Stereo Mix" (or a similar loopback device) appears, right-click it and select "Enable." Make it the default device if you want to capture it by default.  
2. **Use a Virtual Audio Cable (if "Stereo Mix" is not available or doesn't work):**  
   * Install a virtual audio cable software like [VB-Cable](https://vb-audio.com/Cable/) or [Voicemeeter](https://vb-audio.com/Voicemeeter/).  
   * Configure your Windows audio playback to output to the virtual cable's input.  
   * Then, you would need to rename the virtual cable's output device to "Stereo Mix" or modify the client code to target the virtual cable's specific name.

**Using the Client to List Devices (for debugging):**

You can still use the \-list-devices flag to see the names of all available input devices, which can be helpful for debugging if "Stereo Mix" isn't working as expected or if you're using a virtual cable and need its exact name.

### **Client Code Structure**

// client/main.go  
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
	"time"

	"github.com/gordonklaus/portaudio"  
)

// Audio parameters  
const (  
	SampleRate      \= 48000 // Hz  
	Channels        \= 2     // Stereo  
	SampleFormat    \= portaudio.Int16  
	FramesPerBuffer \= 512   // Number of audio frames per buffer  
)

func main() {  
	serverAddrStr := flag.String("server", "127.0.0.1:8080", "Server address (IP:Port) for audio stream")  
	initialVolume := flag.Float64("volume", 1.0, "Initial client-side volume adjustment (0.0 to 1.0)")  
	controlPort := flag.Int("control-port", 8081, "Port to listen for server control messages")  
	listDevices := flag.Bool("list-devices", false, "List available audio input devices and exit.")  
	flag.Parse()

	if \*initialVolume \< 0.0 || \*initialVolume \> 1.0 {  
		log.Fatalf("Initial volume must be between 0.0 and 1.0")  
	}

	// Initialize PortAudio for device listing or streaming  
	err := portaudio.Initialize()  
	if err \!= nil {  
		log.Fatalf("Error initializing PortAudio: %v", err)  
	}  
	defer portaudio.Terminate()

	if \*listDevices {  
		fmt.Println("Available Audio Input Devices:")  
		devices, err := portaudio.Devices()  
		if err \!= nil {  
			log.Fatalf("Error listing devices: %v", err)  
		}  
		for i, info := range devices {  
			if info.MaxInputChannels \> 0 { // Only list input devices  
				fmt.Printf("  \[%d\] %s (Host API: %s)\\n", i, info.Name, info.HostApi.Name)  
			}  
		}  
		return // Exit after listing devices  
	}

	// Use atomic.Value for thread-safe volume adjustment  
	var currentClientVolume atomic.Value  
	currentClientVolume.Store(\*initialVolume)

	// Resolve server address for audio stream  
	serverAddr, err := net.ResolveUDPAddr("udp", \*serverAddrStr)  
	if err \!= nil {  
		log.Fatalf("Error resolving server address: %v", err)  
	}

	// Create UDP connection for audio stream  
	audioConn, err := net.DialUDP("udp", nil, serverAddr)  
	if err \!= nil {  
		log.Fatalf("Error creating UDP audio connection: %v", err)  
	}  
	defer audioConn.Close()

	// Start goroutine to listen for control messages from server  
	go func() {  
		controlAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", \*controlPort))  
		if err \!= nil {  
			log.Printf("Error resolving control listen address: %v", err)  
			return  
		}  
		controlConn, err := net.ListenUDP("udp", controlAddr)  
		if err \!= nil {  
			log.Printf("Error listening on control UDP: %v", err)  
			return  
		}  
		defer controlConn.Close()

		log.Printf("Client control listener started on :%d", \*controlPort)

		controlBuffer := make(\[\]byte, 8\) // For float64 volume  
		for {  
			n, \_, err := controlConn.ReadFromUDP(controlBuffer)  
			if err \!= nil {  
				log.Printf("Error reading control UDP packet: %v", err)  
				continue  
			}  
			if n \== 8 {  
				var receivedVolume float64  
				buf := bytes.NewReader(controlBuffer\[:n\])  
				err \= binary.Read(buf, binary.LittleEndian, \&receivedVolume)  
				if err \!= nil {  
					log.Printf("Error decoding received volume: %v", err)  
					continue  
				}  
				if receivedVolume \>= 0.0 && receivedVolume \<= 1.0 {  
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

	// Open audio input stream  
	var stream \*portaudio.Stream  
	inputBuffer := make(\[\]int16, FramesPerBuffer\*Channels) // 16-bit stereo samples

	// Attempt to find and use "Stereo Mix"  
	stereoMixFound := false  
	devices, err := portaudio.Devices()  
	if err \!= nil {  
		log.Printf("Warning: Error listing devices: %v. Will try default input.", err)  
	} else {  
		var stereoMixDevice \*portaudio.DeviceInfo  
		for \_, info := range devices {  
			// Case-insensitive search for "Stereo Mix" or similar  
			if info.MaxInputChannels \> 0 && strings.Contains(strings.ToLower(info.Name), "stereo mix") {  
				stereoMixDevice \= info  
				stereoMixFound \= true  
				break  
			}  
		}

		if stereoMixFound {  
			log.Printf("Found 'Stereo Mix' device: %s. Attempting to use it.", stereoMixDevice.Name)  
			param := portaudio.StreamParameters{  
				Input: portaudio.StreamDeviceParameters{  
					Device:   stereoMixDevice,  
					Channels: Channels,  
					Latency:  stereoMixDevice.DefaultLowInputLatency,  
				},  
				SampleRate:      SampleRate,  
				FramesPerBuffer: FramesPerBuffer,  
			}  
			stream, err \= portaudio.OpenStream(param, inputBuffer)  
		}  
	}

	// If Stereo Mix not found or failed to open, fall back to default  
	if \!stereoMixFound || err \!= nil {  
		if \!stereoMixFound {  
			log.Println("Warning: 'Stereo Mix' device not found. Falling back to default input device.")  
		} else {  
			log.Printf("Warning: Failed to open 'Stereo Mix' stream: %v. Falling back to default input device.", err)  
		}  
		stream, err \= portaudio.OpenDefaultStream(Channels, 0, SampleRate, FramesPerBuffer, inputBuffer)  
	}

	if err \!= nil {  
		log.Fatalf("Error opening input stream (after trying Stereo Mix and default): %v", err)  
	}  
	defer stream.Close()

	// Start the stream  
	err \= stream.Start()  
	if err \!= nil {  
		log.Fatalf("Error starting input stream: %v", err)  
	}  
	defer stream.Stop()

	fmt.Printf("Client started. Streaming audio to %s\\n", \*serverAddrStr)  
	fmt.Printf("Initial client-side volume: %.2f. Listening for server volume control on port %d\\n", currentClientVolume.Load().(float64), \*controlPort)  
	if stereoMixFound {  
		fmt.Println("Attempting to use 'Stereo Mix' for audio input.")  
	} else {  
		fmt.Println("Using default input device (could not find 'Stereo Mix').")  
	}  
	fmt.Println("Press Ctrl+C to stop.")

	// Buffer for sending data  
	sendBuffer := new(bytes.Buffer)

	for {  
		// Read audio frames from input device  
		err \= stream.Read()  
		if err \!= nil {  
			log.Printf("Error reading from stream: %v", err)  
			continue  
		}

		sendBuffer.Reset() // Clear buffer for new data

		// Get current volume  
		vol := currentClientVolume.Load().(float64)

		// Apply volume adjustment and write to buffer  
		for i := 0; i \< len(inputBuffer); i++ {  
			// Apply volume: float64(sample) \* volume  
			adjustedSample := int16(float64(inputBuffer\[i\]) \* vol)  
			err \= binary.Write(sendBuffer, binary.LittleEndian, adjustedSample)  
			if err \!= nil {  
				log.Printf("Error writing sample to buffer: %v", err)  
				continue  
			}  
		}

		// Send the audio buffer over UDP  
		\_, err \= audioConn.Write(sendBuffer.Bytes())  
		if err \!= nil {  
			log.Printf("Error sending UDP packet: %v", err)  
			// Small delay to prevent busy-looping on network errors  
			time.Sleep(10 \* time.Millisecond)  
		}  
	}  
}

### **Client Build and Run Instructions**

1. **Create a directory** for your client application (e.g., audio-streamer/client).  
2. **Save the code** above as main.go inside that directory.  
3. **Initialize Go module and install dependencies:**  
   cd audio-streamer/client  
   go mod init audio-client  
   go get github.com/gordonklaus/portaudio

4. **Build the client:**  
   go build \-o audio-client

5. **Run the client:**  
   ./audio-client \-server \<SERVER\_IP\>:8080 \-volume 0.8 \-control-port 8081

   * Replace \<SERVER\_IP\> with the actual IP address of your server machine on the local network.  
   * Adjust \-volume for initial client-side volume.  
   * \-control-port specifies the port the client will listen on for volume control commands from the server. Ensure this port is not blocked by a firewall.  
   * The client will now automatically attempt to find and use "Stereo Mix". If it cannot find it, it will fall back to the default input device.

## **Server Application: audio-server**

The server application will listen for UDP packets, receive audio frames, apply volume adjustments, and play the audio through the default output device. It also provides an interface to send volume control commands to the client.

### **Server Code Structure**

// server/main.go  
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
	SampleRate      \= 48000 // Hz  
	Channels        \= 2     // Stereo  
	SampleFormat    \= portaudio.Int16  
	FramesPerBuffer \= 512   // Number of audio frames per buffer  
	PacketSize      \= FramesPerBuffer \* Channels \* 2 // 2 bytes per int16 sample  
)

func main() {  
	listenPort := flag.Int("port", 8080, "Port to listen for audio stream")  
	serverVolume := flag.Float64("volume", 1.0, "Server-side volume adjustment (0.0 to 1.0)")  
	clientControlAddrStr := flag.String("client-control-addr", "", "Client address (IP:Port) for sending control messages (e.g., 127.0.0.1:8081)")  
	flag.Parse()

	if \*serverVolume \< 0.0 || \*serverVolume \> 1.0 {  
		log.Fatalf("Server volume must be between 0.0 and 1.0")  
	}

	// Resolve UDP address to listen on for audio stream  
	audioAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", \*listenPort))  
	if err \!= nil {  
		log.Fatalf("Error resolving audio listen address: %v", err)  
	}

	// Create UDP listener for audio stream  
	audioConn, err := net.ListenUDP("udp", audioAddr)  
	if err \!= nil {  
		log.Fatalf("Error listening on UDP for audio: %v", err)  
	}  
	defer audioConn.Close()

	fmt.Printf("Server started. Listening for audio on UDP port %d with server volume %.2f\\n", \*listenPort, \*serverVolume)  
	fmt.Println("Waiting for audio stream...")  
	fmt.Println("Press Ctrl+C to stop.")

	// Handle client control if address is provided  
	if \*clientControlAddrStr \!= "" {  
		clientControlAddr, err := net.ResolveUDPAddr("udp", \*clientControlAddrStr)  
		if err \!= nil {  
			log.Fatalf("Error resolving client control address: %v", err)  
		}  
		// Create a UDP connection for sending control messages  
		controlConn, err := net.DialUDP("udp", nil, clientControlAddr)  
		if err \!= nil {  
			log.Fatalf("Error creating UDP control connection: %v", err)  
		}  
		defer controlConn.Close()

		fmt.Printf("Ready to send client volume control to %s\\n", \*clientControlAddrStr)  
		fmt.Println("Enter new client volume (0.0-1.0) and press Enter:")

		// Goroutine to read volume from stdin and send to client  
		go func() {  
			reader := bufio.NewReader(os.Stdin)  
			for {  
				fmt.Print("\> ")  
				input, \_ := reader.ReadString('\\n')  
				input \= input\[:len(input)-1\] // Remove newline

				newVolume, err := strconv.ParseFloat(input, 64\)  
				if err \!= nil {  
					fmt.Println("Invalid input. Please enter a number between 0.0 and 1.0.")  
					continue  
				}  
				if newVolume \< 0.0 || newVolume \> 1.0 {  
					fmt.Println("Volume must be between 0.0 and 1.0.")  
					continue  
				}

				// Convert float64 to byte slice  
				buf := new(bytes.Buffer)  
				err \= binary.Write(buf, binary.LittleEndian, newVolume)  
				if err \!= nil {  
					log.Printf("Error encoding volume for sending: %v", err)  
					continue  
				}

				\_, err \= controlConn.Write(buf.Bytes())  
				if err \!= nil {  
					log.Printf("Error sending client volume control: %v", err)  
				} else {  
					fmt.Printf("Sent client volume: %.2f\\n", newVolume)  
				}  
			}  
		}()  
	}

	// Initialize PortAudio  
	err \= portaudio.Initialize()  
	if err \!= nil {  
		log.Fatalf("Error initializing PortAudio: %v", err)  
	}  
	defer portaudio.Terminate()

	// Create output stream  
	outputBuffer := make(\[\]int16, FramesPerBuffer\*Channels) // 16-bit stereo samples  
	stream, err := portaudio.OpenDefaultStream(0, Channels, SampleRate, FramesPerBuffer, outputBuffer)  
	if err \!= nil {  
		log.Fatalf("Error opening default output stream: %v", err)  
	}  
	defer stream.Close()

	// Start the stream  
	err \= stream.Start()  
	if err \!= nil {  
		log.Fatalf("Error starting output stream: %v", err)  
	}  
	defer stream.Stop()

	// Buffer for receiving data  
	receiveBuffer := make(\[\]byte, PacketSize)

	for {  
		// Read UDP packet  
		n, \_, err := audioConn.ReadFromUDP(receiveBuffer)  
		if err \!= nil {  
			log.Printf("Error reading UDP packet: %v", err)  
			// Small delay to prevent busy-looping on network errors  
			time.Sleep(10 \* time.Millisecond)  
			continue  
		}

		if n \!= PacketSize {  
			log.Printf("Received packet of unexpected size: %d bytes (expected %d)", n, PacketSize)  
			continue  
		}

		// Read int16 samples from byte buffer  
		reader := bytes.NewReader(receiveBuffer\[:n\])  
		for i := 0; i \< len(outputBuffer); i++ {  
			var sample int16  
			err \= binary.Read(reader, binary.LittleEndian, \&sample)  
			if err \!= nil {  
				log.Printf("Error reading sample from buffer: %v", err)  
				break  
			}  
			// Apply server-side volume adjustment  
			outputBuffer\[i\] \= int16(float64(sample) \* \*serverVolume)  
		}

		// Write audio frames to output device  
		err \= stream.Write()  
		if err \!= nil {  
			log.Printf("Error writing to stream: %v", err)  
		}  
	}  
}

### **Server Build and Run Instructions**

1. **Create a directory** for your server application (e.g., audio-streamer/server).  
2. **Save the code** above as main.go inside that directory.  
3. **Initialize Go module and install dependencies:**  
   cd audio-streamer/server  
   go mod init audio-server  
   go get github.com/gordonklaus/portaudio

4. **Build the server:**  
   go build \-o audio-server

5. **Run the server:**  
   ./audio-server \-port 8080 \-volume 1.0 \-client-control-addr \<CLIENT\_IP\>:8081

   * Replace \<CLIENT\_IP\> with the actual IP address of your client machine on the local network. This flag is optional; if omitted, the server will not attempt to send client volume control messages.  
   * Adjust \-port if you want to use a different port for audio streaming.  
   * Adjust \-volume for server-side playback volume.  
   * Ensure the client's control port (8081 in this example) is accessible from the server.

## **How to Run the Applications on a Local Network**

1. **Install PortAudio** development libraries on both your client and server systems (see "Prerequisites" section).  
2. **Build both the client and server** applications as described above.  
3. **Find your Local IP Address:**  
   * **Windows:** Open Command Prompt and type ipconfig. Look for "IPv4 Address" under your active network adapter (e.g., Ethernet adapter, Wireless LAN adapter).  
   * **macOS:** Open Terminal and type ifconfig or ip addr. Look for inet address under your active network interface (e.g., en0, en1, wlan0).  
   * **Linux:** Open Terminal and type ip a or ifconfig. Look for inet address under your active network interface (e.g., eth0, wlan0).  
   * The IP address will typically be in the range 192.168.x.x or 10.x.x.x.  
4. **Start the server first:**  
   \# On the server machine, replace \<CLIENT\_IP\> with the client's actual local IP  
   ./audio-server \-port 8080 \-volume 1.0 \-client-control-addr \<CLIENT\_IP\>:8081

5. **Then, start the client:**  
   \# On the client machine, replace \<SERVER\_IP\> with the server's actual local IP  
   ./audio-client \-server \<SERVER\_IP\>:8080 \-volume 0.8 \-control-port 8081

   * Ensure your firewall on both machines allows UDP traffic on ports 8080 (audio) and 8081 (control).

## **Limitations and Potential Improvements**

* **True Desktop Audio Capture (Platform-Specific):** While the client now attempts to use "Stereo Mix", its availability and functionality depend on your specific sound card drivers and Windows configuration. For a more direct and robust solution, exploring Go bindings for WASAPI loopback capture or CGO would be necessary.  
* **Error Handling and Resilience:** The current implementation has basic error logging but no sophisticated error recovery for network issues (e.g., re-establishing connection, handling packet loss gracefully).  
* **Buffering:** The server plays audio immediately as it receives it. A more robust solution would involve a jitter buffer on the server to absorb network latency variations and ensure smoother playback.  
* **Audio Codecs/Compression:** While raw PCM is lossless, it's bandwidth-intensive. For lower bandwidth scenarios, consider implementing a lossless audio codec like FLAC or Opus (in its lossless mode). This would require adding encoding/decoding steps on the client/server respectively.  
* **Network Congestion Control:** For production use, you might need mechanisms to adapt to network conditions, such as dynamically adjusting buffer sizes or bitrates.  
* **Authentication/Encryption:** The current setup has no security. For sensitive audio, you would need to add authentication and encryption (e.g., using DTLS over UDP).  
* **Server Control UI:** The server's client volume control is currently via standard input. For a more user-friendly experience, a simple TUI (Text User Interface) or web UI could be implemented.