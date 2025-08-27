# CLI Audio Streamer

A simple command-line tool for streaming audio from a client to a server over UDP.

## About

This project consists of three main components:

- **Client**: Captures system audio (loopback from speakers/headphones) using CPAL in Rust and streams it to a server.
- **Server**: Receives the audio stream and plays it on the default output device.
- **Mock Client**: A simple client that sends a simulated audio stream, useful for testing the server.

## Getting Started

### Prerequisites

- Rust 1.70 or higher (for client)
- Go 1.15 or higher (for server and mock-client)
- CPAL dependencies (installed automatically via Cargo)

To install Rust:

```sh
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

On Linux, you may need to install ALSA development libraries:

```sh
# Ubuntu/Debian
sudo apt-get install libasound2-dev
# Fedora
sudo dnf install alsa-lib-devel
```

### Building

To build the client (Rust), server, and mock client (Go), run the following commands:

```sh
cd client && cargo build --release
cd ../server && go build
cd ../mock-client && go build
```

The client binary will be at `client/target/release/audio-client`.

## Usage

### Streaming System Audio (Loopback)

The client automatically attempts to capture system audio by detecting loopback devices (e.g., "Stereo Mix" on Windows, "BlackHole" on macOS). If no loopback device is found, it falls back to the default input device.

#### Windows

The client will automatically find and use "Stereo Mix" if available, which captures system playback. No additional setup required.

If you prefer to use a virtual audio cable:

1.  **Install VB-CABLE**: Download and install [VB-CABLE](https://vb-audio.com/Cable/index.htm).
2.  **Set Default Playback Device**: Set `CABLE Input` as your default playback device.
3.  **Run the Client**: Use the device name:
    ```sh
    ./client/target/release/audio-client --device-name "CABLE Output (VB-Audio Virtual Cable)" --server <server-ip>
    ```

#### macOS

Install [BlackHole](https://github.com/ExistentialAudio/BlackHole) and route system audio through it. The client will detect it automatically, or specify:

```sh
./client/target/release/audio-client --device-name "BlackHole 2ch" --server <server-ip>
```

#### Linux

The client uses PulseAudio or ALSA loopback if available. No additional setup usually required.

### Server

To start the server, run the following command:

```sh
./server/audio-server
```

### Client

To start the client, run the following command:

```sh
./client/target/release/audio-client --server <server-ip>
```

For example:

```sh
./client/target/release/audio-client --server 127.0.0.1
```

#### Client Options

- `--server <ip>`: Server IP address (default: 127.0.0.1)
- `--volume <0.0-1.0>`: Initial volume (default: 1.0)
- `--control-port <port>`: Port for server control messages (default: 8081)
- `--list-devices`: List available input devices and exit
- `--device-name <name>`: Use specific device by name
- `--device-index <index>`: Use specific device by index

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
