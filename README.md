# Cross-Platform CAN Bus Reader

A Go application for reading CAN bus data from USB devices on both Linux and Windows platforms.

## Features

- **Cross-platform support**: Works on Linux (SocketCAN) and Windows (ControlCAN.dll)
- **Real-time CAN frame reading**: Reads CAN frames with configurable timeout
- **CAN frame writing**: Can send CAN frames back to the bus
- **Filter support**: Configurable acceptance filters
- **Simulation mode**: Works without physical hardware for testing
- **Command-line interface**: Easy to use with command-line options
- **Automatic fallback**: Falls back to simulation if hardware/DLL not available

## Installation

### Prerequisites

- Go 1.21 or later
- On Linux: SocketCAN support and `ip` command (usually available)
- On Windows: ControlCAN.dll (provided with CAN hardware drivers)

### Build

```bash
# Build for current platform
go build -o canbus-reader

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o canbus-reader-linux

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o canbus-reader-windows.exe
```

## Usage

```bash
# Basic usage (defaults to can0 interface, 500kbps)
./canbus-reader

# Specify interface and bitrate
./canbus-reader -interface can0 -bitrate 500000

# Show help
./canbus-reader -help
```

### Command-line Options

- `-interface`: CAN interface name (default: "can0")
  - On Linux: "can0", "can1", "vcan0"
  - On Windows: "0" (device index 0, CAN channel 0), "USBCAN2_0_0"
- `-bitrate`: CAN bus bitrate in bps (default: 500000)
- `-help`: Show help message

## Platform-Specific Details

### Linux (SocketCAN)

The Linux implementation uses SocketCAN subsystem. For real hardware:

1. Load CAN kernel modules:
   ```bash
   sudo modprobe can
   sudo modprobe can_raw
   sudo modprobe can_dev
   ```

2. Bring up CAN interface:
   ```bash
   sudo ip link set can0 up type can bitrate 500000
   ```

3. Run the application:
   ```bash
   sudo ./canbus-reader -interface can0
   ```

Common interface names: `can0`, `can1`, `vcan0` (virtual)

### Windows (ControlCAN.dll)

The Windows implementation uses ControlCAN.dll (commonly provided with ZLG/USBCAN devices). For real hardware:

1. Install device drivers (usually includes ControlCAN.dll)
2. Place ControlCAN.dll in the same directory as the executable or in system PATH
3. Connect CAN-USB device
4. Run the application:
   ```cmd
   canbus-reader.exe -interface 0
   ```

**Interface naming on Windows:**
- Simple number: `0` = device index 0, CAN channel 0
- Extended format: `USBCAN2_0_0` = USBCAN2 device, index 0, CAN channel 0

**Note on DLL architecture:** The application is built for 64-bit Windows. If you have a 32-bit ControlCAN.dll, you may need to:
1. Use 32-bit Go (GOARCH=386) to build a 32-bit executable
2. Or obtain a 64-bit version of ControlCAN.dll from your device manufacturer

### Simulation Mode

If no CAN hardware is detected or ControlCAN.dll cannot be loaded, the application automatically falls back to simulation mode and generates test CAN frames. This is useful for testing without physical hardware.

## CAN Frame Format

The application displays CAN frames in the format:
```
[frame#] TYPE ID: XXX DLC: N Data: XX XX XX ...
```

Where:
- `frame#`: Sequential frame number
- `TYPE`: `DATA` for data frames, `RTR` for remote frames
- `ID`: CAN identifier (3 hex digits for standard, 8 for extended)
- `DLC`: Data Length Code (0-8)
- `Data`: Payload data in hex

## Architecture

The application uses Go's build tags for platform-specific implementations:

- `can_linux.go`: Linux SocketCAN implementation (build tag: `linux`)
- `can_windows.go`: Windows ControlCAN.dll implementation (build tag: `windows`)
- `controlcan_windows.go`: ControlCAN.dll wrapper with Go bindings
- `can_stub.go`: Stub implementation for other platforms with simulation capability

The main interface `CANReader` defines methods for opening/closing interfaces, reading/writing frames, and setting filters.

## ControlCAN.dll Integration

The Windows implementation includes a complete Go wrapper for ControlCAN.dll that mirrors the C++ API shown in the provided code. Key features:

1. **Direct DLL binding**: Uses `syscall` package to load and call ControlCAN.dll functions
2. **Structure mapping**: Go structs match `VCI_INIT_CONFIG`, `VCI_CAN_OBJ` from the DLL
3. **Automatic conversion**: Converts between Go `CANFrame` and `VCI_CAN_OBJ` structures
4. **Error handling**: Graceful fallback to simulation if DLL loading fails

### DLL Loading Issues

If you encounter "not a valid Win32 application" error:
1. Check if ControlCAN.dll is 32-bit or 64-bit
2. Ensure Go build architecture matches DLL architecture
3. Try building with `GOARCH=386` for 32-bit DLLs:
   ```bash
   GOOS=windows GOARCH=386 go build -o canbus-reader-32bit.exe
   ```

## Testing

Run basic tests:

```bash
# Test compilation
go build ./...

# Run in simulation mode (no hardware needed)
go run .

# Test Windows implementation (with simulation fallback)
go run . -interface 0 -bitrate 500000
```

## Extending for Other Platforms

To add support for another platform:

1. Create a new file with appropriate build tag (e.g., `//go:build darwin`)
2. Implement the `CANReader` interface
3. Update `newPlatformCANReader()` to return your implementation

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.