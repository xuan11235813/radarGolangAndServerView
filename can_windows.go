//go:build windows

package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WindowsCANReader implements CANReader for Windows using ControlCAN.dll
type WindowsCANReader struct {
	cc           *ControlCAN
	devType      uint32
	devIndex     uint32
	canIndex     uint32
	connected    bool
	receiveMutex sync.Mutex
	receiveBuf   []VCI_CAN_OBJ
	simulated    bool // Fallback to simulation if DLL fails

	// For continuous reading
	frameChan    chan *CANFrame
	stopChan     chan struct{}
	reading      bool
	readingMutex sync.Mutex
}

// newPlatformCANReader creates a Windows CAN reader
func newPlatformCANReader() (CANReader, error) {
	// Try to load ControlCAN.dll
	cc, err := NewControlCAN("ControlCAN.dll")
	if err != nil {
		log.Printf("Failed to load ControlCAN.dll: %v, using simulation mode", err)
		return &WindowsCANReader{
			simulated: true,
			connected: false,
			frameChan: make(chan *CANFrame, 1000), // Buffered channel for frames
			stopChan:  make(chan struct{}),
		}, nil
	}

	return &WindowsCANReader{
		cc:         cc,
		simulated:  false,
		connected:  false,
		devType:    VCI_USBCAN2,                // Default to USBCAN2
		devIndex:   0,                          // Default device index
		canIndex:   0,                          // Default CAN channel
		receiveBuf: make([]VCI_CAN_OBJ, 2500),  // Buffer for received frames
		frameChan:  make(chan *CANFrame, 1000), // Buffered channel for frames
		stopChan:   make(chan struct{}),
	}, nil
}

// Open opens a CAN interface on Windows using ControlCAN.dll
func (r *WindowsCANReader) Open(interfaceName string, bitrate int) error {
	if r.simulated {
		log.Printf("Opening Windows CAN interface in simulation mode: %s at %d bps", interfaceName, bitrate)
		log.Printf("Simulation mode enabled - generating test CAN frames")
		r.connected = true
		return nil
	}

	// Parse interface name to get device index and CAN index
	// Expected format: "USBCAN2_0_0" or just "0" for default
	devIndex, canIndex := parseInterfaceName(interfaceName)
	r.devIndex = uint32(devIndex)
	r.canIndex = uint32(canIndex)

	// Convert bitrate to Timing0/Timing1 values
	timing0, timing1 := bitrateToTiming(bitrate)

	// Initialize CAN configuration (similar to C++ code)
	initConfig := VCI_INIT_CONFIG{
		AccCode: 0x00000000, // Accept all frames
		AccMask: 0xFFFFFFFF, // Accept all frames
		Filter:  1,          // Enable filter
		Timing0: timing0,
		Timing1: timing1,
		Mode:    0, // 0 for normal, 1 for listen-only
	}

	log.Printf("Opening CAN device: Type=%d, Index=%d, CAN=%d, Bitrate=%d",
		r.devType, r.devIndex, r.canIndex, bitrate)

	// Open device
	if r.cc.OpenDevice(r.devType, r.devIndex) != 1 {
		return fmt.Errorf("failed to open CAN device")
	}

	// Initialize CAN channel
	if r.cc.InitCAN(r.devType, r.devIndex, r.canIndex, &initConfig) != 1 {
		r.cc.CloseDevice(r.devType, r.devIndex)
		return fmt.Errorf("failed to initialize CAN channel")
	}

	// Start CAN
	if r.cc.StartCAN(r.devType, r.devIndex, r.canIndex) != 1 {
		r.cc.CloseDevice(r.devType, r.devIndex)
		return fmt.Errorf("failed to start CAN")
	}

	r.connected = true
	log.Printf("CAN interface opened successfully")
	return nil
}

// Close closes the CAN interface
func (r *WindowsCANReader) Close() error {
	if !r.connected {
		return nil
	}

	// Stop continuous reading if active
	r.StopReading()

	if !r.simulated && r.cc != nil {
		// Try to close device, but don't fail if it's already disconnected
		r.cc.CloseDevice(r.devType, r.devIndex)
		// Note: Don't close the DLL (r.cc.Close()) as we may need to reuse it for reconnection
	}

	r.connected = false
	log.Printf("Windows CAN interface closed")
	return nil
}

// IsConnected returns the current connection status
func (r *WindowsCANReader) IsConnected() bool {
	return r.connected
}

// Read reads a CAN frame with timeout (blocking, for backward compatibility)
func (r *WindowsCANReader) Read(timeout time.Duration) (*CANFrame, error) {
	if !r.connected {
		return nil, fmt.Errorf("CAN interface not connected")
	}

	if r.simulated {
		// Simulated mode
		select {
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout")
		case <-time.After(50 * time.Millisecond):
			return generateSimulatedFrame(), nil
		}
	}

	// Real implementation using ControlCAN.dll
	waitTime := uint32(timeout.Milliseconds())
	if waitTime == 0 {
		waitTime = 100 // Default 100ms
	}

	r.receiveMutex.Lock()
	defer r.receiveMutex.Unlock()

	// Receive frames
	received := r.cc.Receive(r.devType, r.devIndex, r.canIndex, &r.receiveBuf[0], uint32(len(r.receiveBuf)), waitTime)
	if received == 0 {
		return nil, fmt.Errorf("timeout")
	}

	if received > 0 {
		// Return the first frame
		frame := r.receiveBuf[0].ToCANFrame()

		// Shift buffer if we have more frames
		if received > 1 {
			copy(r.receiveBuf, r.receiveBuf[1:received])
		}

		return frame, nil
	}

	return nil, fmt.Errorf("receive error")
}

// StartReading starts continuous non-blocking reading and returns a channel for frames
func (r *WindowsCANReader) StartReading() (<-chan *CANFrame, error) {
	if !r.connected {
		return nil, fmt.Errorf("CAN interface not connected")
	}

	r.readingMutex.Lock()
	defer r.readingMutex.Unlock()

	if r.reading {
		return r.frameChan, nil // Already reading
	}

	r.reading = true

	// Start continuous reading goroutine
	go r.continuousRead()

	return r.frameChan, nil
}

// StopReading stops the continuous reading goroutine
func (r *WindowsCANReader) StopReading() {
	r.readingMutex.Lock()
	defer r.readingMutex.Unlock()

	if !r.reading {
		return
	}

	close(r.stopChan)
	r.reading = false

	// Reset stop channel for future use
	r.stopChan = make(chan struct{})
}

// continuousRead continuously reads CAN frames and sends them to the channel
func (r *WindowsCANReader) continuousRead() {
	if r.simulated {
		r.continuousReadSimulated()
		return
	}

	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	// Real hardware reading
	for {
		select {
		case <-r.stopChan:
			return
		default:
			// Use very short timeout for non-blocking check
			waitTime := uint32(10) // 10ms timeout

			r.receiveMutex.Lock()
			received := r.cc.Receive(r.devType, r.devIndex, r.canIndex, &r.receiveBuf[0], uint32(len(r.receiveBuf)), waitTime)
			r.receiveMutex.Unlock()

			// Check for USB disconnection errors
			// Error codes: 0xFFFFFFFF (-1) or specific error codes like 433 indicate disconnection
			if received == 0xFFFFFFFF || (received > 0x1000 && received < 0x10000) {
				// This is an error code, not a frame count - USB likely disconnected
				consecutiveErrors++
				log.Printf("CAN receive error code: %d (0x%X), consecutive errors: %d", received, received, consecutiveErrors)

				if consecutiveErrors >= maxConsecutiveErrors {
					log.Printf("Device disconnected (error code %d), signaling for reconnection", received)
					// Signal disconnection by sending nil frame
					select {
					case r.frameChan <- nil:
						// Nil frame sent to signal disconnection
					default:
						// Channel full, close and reopen
					}
					return // Exit the goroutine, main loop will handle reconnection
				}
				time.Sleep(100 * time.Millisecond) // Wait before retry
				continue
			}

			// Reset error count on successful receive (including timeout with 0)
			consecutiveErrors = 0

			if received > 0 && received < 0x1000 {
				// Process all received frames (valid range)
				for i := 0; i < int(received); i++ {
					frame := r.receiveBuf[i].ToCANFrame()

					// Send frame to channel (non-blocking)
					select {
					case r.frameChan <- frame:
						// Frame sent successfully
					default:
						// Channel full, drop frame (could log this)
						log.Printf("Warning: Frame channel full, dropping frame ID: %X", frame.ID)
					}
				}
			}

			// Small sleep to prevent CPU spinning
			time.Sleep(1 * time.Millisecond)
		}
	}
}

// continuousReadSimulated generates simulated frames
func (r *WindowsCANReader) continuousReadSimulated() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			frame := generateSimulatedFrame()

			// Send frame to channel (non-blocking)
			select {
			case r.frameChan <- frame:
				// Frame sent successfully
			default:
				// Channel full, drop frame
			}
		}
	}
}

// Write writes a CAN frame
func (r *WindowsCANReader) Write(frame *CANFrame) error {
	if frame == nil {
		return fmt.Errorf("frame is nil")
	}

	if !r.connected {
		return fmt.Errorf("CAN interface not connected")
	}

	if r.simulated {
		// Log simulated write
		dataHex := ""
		for i, b := range frame.Data {
			if i > 0 {
				dataHex += " "
			}
			dataHex += fmt.Sprintf("%02X", b)
		}
		log.Printf("SIMULATED WRITE: ID: %X, DLC: %d, Data: %s",
			frame.ID, frame.Length, dataHex)
		return nil
	}

	// Convert to VCI_CAN_OBJ
	canObj := frame.ToVCI_CAN_OBJ()

	// Transmit frame
	if r.cc.Transmit(r.devType, r.devIndex, r.canIndex, canObj, 1) != 1 {
		return fmt.Errorf("failed to transmit CAN frame")
	}

	log.Printf("CAN frame transmitted: ID: %X, DLC: %d", frame.ID, frame.Length)
	return nil
}

// SetFilter sets acceptance filters
func (r *WindowsCANReader) SetFilter(filters []Filter) error {
	log.Printf("Setting %d filter(s) on Windows CAN interface", len(filters))

	for i, filter := range filters {
		log.Printf("  Filter %d: ID=0x%X, Mask=0x%X, Extended=%v",
			i+1, filter.ID, filter.Mask, filter.Extend)
	}

	// Note: In ControlCAN.dll, filters are set via VCI_INIT_CONFIG
	// We would need to reinitialize the CAN channel with new filters
	// For simplicity, we just log them for now

	return nil
}

// Helper functions

// parseInterfaceName parses interface name like "USBCAN2_0_0" or "0"
func parseInterfaceName(name string) (devIndex, canIndex int) {
	// Default values
	devIndex = 0
	canIndex = 0

	// Try to parse as simple number
	if idx, err := strconv.Atoi(name); err == nil {
		devIndex = idx
		return
	}

	// Try to parse as "USBCAN2_0_0" format
	parts := strings.Split(name, "_")
	if len(parts) >= 2 {
		if idx, err := strconv.Atoi(parts[1]); err == nil {
			devIndex = idx
		}
	}
	if len(parts) >= 3 {
		if idx, err := strconv.Atoi(parts[2]); err == nil {
			canIndex = idx
		}
	}

	return
}

// bitrateToTiming converts bitrate to Timing0/Timing1 values
// This is a simplified conversion - actual values depend on CAN controller
func bitrateToTiming(bitrate int) (timing0, timing1 uint8) {
	// Common bitrate settings for ControlCAN devices
	// These values are examples and may need adjustment for specific hardware
	switch bitrate {
	case 10000: // 10 kbps
		return 0x31, 0x1C
	case 20000: // 20 kbps
		return 0x18, 0x1C
	case 50000: // 50 kbps
		return 0x09, 0x1C
	case 100000: // 100 kbps
		return 0x04, 0x1C
	case 125000: // 125 kbps
		return 0x03, 0x1C
	case 250000: // 250 kbps
		return 0x01, 0x1C
	case 500000: // 500 kbps
		return 0x00, 0x1C
	case 800000: // 800 kbps
		return 0x00, 0x16
	case 1000000: // 1 Mbps
		return 0x00, 0x14
	default: // Default to 500 kbps
		return 0x00, 0x1C
	}
}

// generateSimulatedFrame creates a test CAN frame for simulation
func generateSimulatedFrame() *CANFrame {
	// Generate realistic CAN frames
	frames := []struct {
		id   uint32
		data []byte
	}{
		{0x100, []byte{0x01, 0x02, 0x03, 0x04}},
		{0x101, []byte{0x10, 0x20, 0x30}},
		{0x200, []byte{0xFF, 0xFE, 0xFD, 0xFC}},
		{0x201, []byte{0x55, 0xAA, 0x55, 0xAA}},
		{0x300, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}},
	}

	idx := int(time.Now().UnixNano()/100000000) % len(frames)

	return &CANFrame{
		ID:        frames[idx].id,
		Data:      frames[idx].data,
		Length:    uint8(len(frames[idx].data)),
		Timestamp: time.Now(),
		Extended:  false,
		Remote:    false,
	}
}
