# CLI Audio Streamer

A simple command-line tool for streaming audio from a client to a server over UDP.

## About

This project consists of three main components:

- **Client**: Captures audio from the default input device (or "Stereo Mix" if available) and streams it to a server.
- **Server**: Receives the audio stream and plays it on the default output device.
- **Mock Client**: A simple client that sends a simulated audio stream, useful for testing the server.

## Getting Started

### Prerequisites

- Go 1.15 or higher
- PortAudio

To install PortAudio on macOS, use Homebrew:

```sh
brew install portaudio
```

### Building

To build the client, server, and mock client, run the following commands:

```sh
cd client && go build
cd ../server && go build
cd ../mock-client && go build
```

## Usage

### Streaming System Audio (Loopback)

To stream the audio that's currently playing on your computer (e.g., from a game or music player), you need to use a virtual audio cable. This tool creates a "loopback" device that captures system playback.

#### Windows

1.  **Install VB-CABLE**: Download and install [VB-CABLE](https://vb-audio.com/Cable/index.htm). This will create two new audio devices:
    *   `CABLE Input` (a playback device)
    *   `CABLE Output` (a recording device)

2.  **Set Default Playback Device**: Set `CABLE Input` as your default playback device. You can do this from the Windows Sound dialog (right-click the speaker icon in the taskbar, click **Sounds**, and go to the **Playback** tab). All system audio will now be routed to the virtual cable.

3.  **Run the Client**: Start the client and tell it to use the `CABLE Output` recording device.
    *   First, list the devices to find the correct name:
        ```sh
        ./client/audio-client -list-devices
        ```
    *   Then, run the client using the device name:
        ```sh
        ./client/audio-client -device-name "CABLE Output (VB-Audio Virtual Cable)"
        ```

#### macOS

On macOS, you can use a free tool like [BlackHole](https://github.com/ExistentialAudio/BlackHole) to achieve the same result. After installing, you can route system audio through the BlackHole device and select it as the input source in the client.

### Server

To start the server, run the following command:

```sh
./server/audio-server
```

### Client

To start the client, run the following command:

```sh
./client/audio-client --server <server-ip>
```

For example:

```sh
./client/audio-client --server 127.0.0.1
```

### Mock Client (for testing)

The mock client sends a simulated audio stream to the server. This is useful for testing the server without a real audio source.

To start the mock client, run the following command:

```sh
./mock-client/mock-client --server <server-ip>
```

For example:

```sh
./mock-client/mock-client --server 127.0.0.1
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
