# CAN Bus Multi-Device 2D Point Viewer

A real-time CAN bus data visualization system with WebSocket server and web-based 2D point display for multiple devices.

## Features

- **Multi-device support**: Monitor multiple CAN interfaces simultaneously
- **Real-time WebSocket server**: Broadcasts 2D point data to web clients on port 1999
- **Interactive web interface**: 
  - Pan (left-click drag) and zoom (mouse wheel) functionality
  - Adaptive grid and axis labels based on zoom level
  - Equal scale for both axes (1 meter = same pixel length)
- **CSV data logging**: Automatically records cluster data with timestamps
- **Cross-platform**: Works on Linux (SocketCAN) and Windows (ControlCAN.dll)

## Architecture

```
CAN Bus → Go Application → WebSocket Server → Web Browser
                ↓
           CSV Logging
```

## Installation

### Prerequisites

- Go 1.21 or later
- On Linux: SocketCAN support
- On Windows: ControlCAN.dll (provided with CAN hardware drivers)

### Build

```bash
# Build for current platform
go build -o canbus-multidevice.exe

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o canbus-multidevice

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o canbus-multidevice.exe
```

## Configuration

Edit `config.json` to specify which CAN devices to monitor:

```json
{
    "devices": [
        "can0",
        "can1"
    ]
}
```

## Usage

1. **Start the application**:
   ```bash
   ./canbus-multidevice.exe
   ```

2. **Open web browser**: Navigate to `http://localhost:1999`

3. **Interact with the visualization**:
   - **Zoom**: Mouse wheel up/down to zoom in/out (centered on cursor)
   - **Pan**: Left-click and drag to move the view
   - **Toggle Grid**: Show/hide grid lines
   - **Toggle Axes**: Show/hide axis labels
   - **Clear Points**: Remove all displayed points
   - **Reset Zoom**: Reset to default view (Longitude: 0-200m, Latitude: -50 to 50m)
   - **Reset All**: Reset everything to initial state
   - **Reconnect All**: Re-establish WebSocket connections

## Coordinate System

- **Longitude (distLong)**: Vertical axis, 0m at bottom to 200m at top
- **Latitude (distLat)**: Horizontal axis, -50m at left to 50m at right
- **Equal scale**: 1 meter has the same pixel length in both directions

## Data Format

### WebSocket Message Format

Each device broadcasts JSON arrays of 2D points:

```json
[
    {"distLong": 50.5, "distLat": -10.2},
    {"distLong": 75.3, "distLat": 15.8}
]
```

### CSV Log Format

Files are named: `YYYY_MM_DD_HH_MM_SS_deviceName.csv`

Columns:
- Timestamp (milliseconds UTC)
- ID (cluster ID)
- DistLong (longitude distance in meters)
- DistLat (latitude distance in meters)
- VrelLong (relative longitudinal velocity)
- VrelLat (relative lateral velocity)
- DynProp (dynamic property)
- RCS (radar cross section)
- DeviceName

## Platform-Specific Details

### Linux (SocketCAN)

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
   sudo ./canbus-multidevice
   ```

### Windows (ControlCAN.dll)

1. Install device drivers (includes ControlCAN.dll)
2. Place ControlCAN.dll in the same directory as the executable
3. Connect CAN-USB device
4. Run the application:
   ```cmd
   canbus-multidevice.exe
   ```

## Files

- `main.go` - Main application with CAN reading and WebSocket server
- `index.html` - Web interface for 2D point visualization
- `config.json` - Device configuration
- `can_linux.go` - Linux SocketCAN implementation
- `can_windows.go` - Windows ControlCAN implementation
- `can_stub.go` - Simulation mode for testing without hardware
- `print_cluster.go` - Cluster data formatting utilities

## WebSocket Endpoints

- `ws://hostname:1999/{deviceName}` - WebSocket endpoint for each device
- `http://hostname:1999/` - Web interface (serves index.html)
- `http://hostname:1999/config.json` - Configuration file

## Adaptive Grid System

The grid and axis labels automatically adjust based on zoom level:

| Zoom Level | Grid Step | Label Step |
|------------|-----------|------------|
| Very zoomed in | 1m | 2m |
| Zoomed in | 2m | 5m |
| Normal | 5m | 10m |
| Zoomed out | 10m | 20m |
| Far zoomed out | 20m | 50m |
| Very far zoomed out | 50m | 100m |
| Default view | 100m | 200m |

## License

MIT License
