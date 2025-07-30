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

### Server

To start the server, run the following command:

```sh
./server/audio-server
```

### Client

To start the client, run the following command:

```sh
./client/audio-client --server <server-ip>:<server-port>
```

For example:

```sh
./client/audio-client --server 127.0.0.1:8080
```

### Mock Client (for testing)

The mock client sends a simulated audio stream to the server. This is useful for testing the server without a real audio source.

To start the mock client, run the following command:

```sh
./mock-client/mock-client <server-ip>:<server-port>
```

For example:

```sh
./mock-client/mock-client 127.0.0.1:8080
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
