use clap::Parser;
use cpal::traits::{DeviceTrait, HostTrait, StreamTrait};
use std::sync::{Arc, Mutex};
use tokio::net::UdpSocket;
use byteorder::{LittleEndian, ReadBytesExt};
use std::io::Cursor;

use crate::select_device;

#[derive(Parser)]
#[command(name = "audio-client")]
#[command(about = "Captures system audio and streams over UDP")]
struct Args {
    /// Server IP address
    #[arg(long, default_value = "127.0.0.1")]
    server: String,

    /// Initial client-side volume (0.0 to 1.0)
    #[arg(long, default_value = "1.0")]
    volume: f32,

    /// Port to listen for server control messages
    #[arg(long, default_value = "8081")]
    control_port: u16,

    /// List available audio input devices and exit
    #[arg(long)]
    list_devices: bool,

    /// Name of the audio input device to use
    #[arg(long)]
    device_name: Option<String>,

    /// Index of the audio input device to use
    #[arg(long)]
    device_index: Option<usize>,
}

const SAMPLE_RATE: u32 = 48000;
const CHANNELS: u16 = 2;
const FRAMES_PER_BUFFER: u32 = 512;
const SERVER_AUDIO_PORT: u16 = 8080;


        }
    }
    None
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();

    if args.volume < 0.0 || args.volume > 1.0 {
        eprintln!("Volume must be between 0.0 and 1.0");
        std::process::exit(1);
    }

    let host = cpal::default_host();
    let devices: Vec<_> = host.devices()?.collect();

    if args.list_devices {
        println!("Available Audio Input Devices:");
        for (i, device) in devices.iter().enumerate() {
            if let Ok(configs) = device.supported_input_configs() {
                if configs.count() > 0 {
                    if let Ok(name) = device.name() {
                        println!("  [{}] {} (Host: {})", i, name, host.id().name());
                    }
                }
            }
        }
        return Ok(());
    }

    let selected_device = select_device(&devices, args.device_index, args.device_name.as_deref());

    let device = match selected_device {
        Some(d) => d,
        None => {
            eprintln!("No suitable input device found");
            std::process::exit(1);
        }
    };

    println!("Using audio input: {}", device.name()?);

    let config = device.default_input_config()?;
    let sample_format = config.sample_format();
    let config = cpal::StreamConfig {
        channels: CHANNELS,
        sample_rate: cpal::SampleRate(SAMPLE_RATE),
        buffer_size: cpal::BufferSize::Fixed(FRAMES_PER_BUFFER),
    };

    let volume = Arc::new(Mutex::new(args.volume));
    let server_addr = format!("{}:{}", args.server, SERVER_AUDIO_PORT);
    let socket = UdpSocket::bind("0.0.0.0:0").await?;
    socket.connect(&server_addr).await?;

    let socket_clone = socket.clone();
    let volume_clone = volume.clone();

    // Control listener
    tokio::spawn(async move {
        let control_addr = format!("0.0.0.0:{}", args.control_port);
        let control_socket = match UdpSocket::bind(&control_addr).await {
            Ok(s) => s,
            Err(e) => {
                eprintln!("Error binding control socket: {}", e);
                return;
            }
        };

        println!("Client control listener started on :{}", args.control_port);

        let mut buf = [0u8; 8];
        loop {
            match control_socket.recv_from(&mut buf).await {
                Ok((len, _)) => {
                    if len == 8 {
                        let mut cursor = Cursor::new(&buf);
                        if let Ok(received_volume) = cursor.read_f64::<byteorder::LittleEndian>() {
                            if received_volume >= 0.0 && received_volume <= 1.0 {
                                *volume_clone.lock().unwrap() = received_volume as f32;
                                println!("Client volume updated to: {:.2f}", received_volume);
                            } else {
                                eprintln!("Received invalid volume: {:.2f}", received_volume);
                            }
                        }
                    }
                }
                Err(e) => eprintln!("Error receiving control: {}", e),
            }
        }
    });

    let err_fn = |err| eprintln!("Stream error: {}", err);

    let stream = match sample_format {
        cpal::SampleFormat::F32 => device.build_input_stream(
            &config,
            move |data: &[f32], _: &cpal::InputCallbackInfo| {
                let vol = *volume.lock().unwrap();
                let mut buffer = Vec::new();
                for &sample in data {
                    let adjusted = (sample * vol).clamp(-1.0, 1.0);
                    let int_sample = (adjusted * i16::MAX as f32) as i16;
                    buffer.extend_from_slice(&int_sample.to_le_bytes());
                }
                if !buffer.is_empty() {
                    // Note: In async context, this should be handled differently, but for simplicity
                    // We'll ignore send errors here
                    let _ = socket_clone.try_send(&buffer);
                }
            },
            err_fn,
        )?,
        cpal::SampleFormat::I16 => device.build_input_stream(
            &config,
            move |data: &[i16], _: &cpal::InputCallbackInfo| {
                let vol = *volume.lock().unwrap();
                let mut buffer = Vec::new();
                for &sample in data {
                    let adjusted = ((sample as f32 / i16::MAX as f32) * vol).clamp(-1.0, 1.0);
                    let int_sample = (adjusted * i16::MAX as f32) as i16;
                    buffer.extend_from_slice(&int_sample.to_le_bytes());
                }
                if !buffer.is_empty() {
                    let _ = socket_clone.try_send(&buffer);
                }
            },
            err_fn,
        )?,
        _ => {
            eprintln!("Unsupported sample format: {:?}", sample_format);
            std::process::exit(1);
        }
    };

    stream.play()?;
    println!("Streaming... Press Ctrl+C to stop.");

    // Keep the main thread alive
    tokio::signal::ctrl_c().await?;
    stream.pause()?;
    Ok(())
}