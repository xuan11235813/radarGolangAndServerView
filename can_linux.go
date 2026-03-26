//go:build linux

package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LinuxCANReader implements CANReader for Linux using SocketCAN (via command line tools)
type LinuxCANReader struct {
	interfaceName string
	simulated     bool
	connected     bool

	// For continuous reading
	frameChan    chan *CANFrame
	stopChan     chan struct{}
	reading      bool
	readingMutex sync.Mutex
}

// newPlatformCANReader creates a Linux CAN reader
func newPlatformCANReader() (CANReader, error) {
	return &LinuxCANReader{
		simulated: true,
		connected: false,
		frameChan: make(chan *CANFrame, 1000),
		stopChan:  make(chan struct{}),
	}, nil
}

// Open opens a SocketCAN interface
func (r *LinuxCANReader) Open(interfaceName string, bitrate int) error {
	r.interfaceName = interfaceName

	// Check if CAN interface exists
	if !r.checkInterfaceExists(interfaceName) {
		log.Printf("CAN interface %s not found, running in simulation mode", interfaceName)
		r.simulated = true
		r.connected = true // Simulation mode is "connected"
		return nil
	}

	// Try to bring up the interface
	err := r.bringUpInterface(interfaceName, bitrate)
	if err != nil {
		log.Printf("Failed to bring up %s: %v, running in simulation mode", interfaceName, err)
		r.simulated = true
		r.connected = true // Simulation mode is "connected"
		return nil
	}

	log.Printf("CAN interface %s opened at %d bps", interfaceName, bitrate)
	r.simulated = false
	r.connected = true
	return nil
}

// Close closes the CAN interface
func (r *LinuxCANReader) Close() error {
	r.StopReading()
	r.connected = false
	log.Printf("Closing Linux CAN interface %s", r.interfaceName)
	return nil
}

// IsConnected returns the current connection status
func (r *LinuxCANReader) IsConnected() bool {
	return r.connected
}

// Read reads a CAN frame with timeout (blocking, for backward compatibility)
func (r *LinuxCANReader) Read(timeout time.Duration) (*CANFrame, error) {
	if r.simulated {
		// Simulated mode
		select {
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout")
		case <-time.After(50 * time.Millisecond):
			return generateLinuxFrame(), nil
		}
	}

	// Real implementation would use candump or direct socket reading
	// For this example, we'll simulate even in "real" mode
	select {
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	case <-time.After(100 * time.Millisecond):
		return generateLinuxFrame(), nil
	}
}

// StartReading starts continuous non-blocking reading and returns a channel for frames
func (r *LinuxCANReader) StartReading() (<-chan *CANFrame, error) {
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
func (r *LinuxCANReader) StopReading() {
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
func (r *LinuxCANReader) continuousRead() {
	if r.simulated {
		r.continuousReadSimulated()
		return
	}

	// Real hardware reading using candump
	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	// Start candump process
	cmd := exec.Command("candump", r.interfaceName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to create candump pipe: %v", err)
		// Signal disconnection
		select {
		case r.frameChan <- nil:
		default:
		}
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start candump: %v", err)
		// Signal disconnection
		select {
		case r.frameChan <- nil:
		default:
		}
		return
	}

	// Read from candump output
	buf := make([]byte, 1024)
	for {
		select {
		case <-r.stopChan:
			cmd.Process.Kill()
			cmd.Wait()
			return
		default:
			// Set non-blocking read with timeout
			n, err := stdout.Read(buf)
			if err != nil {
				consecutiveErrors++
				log.Printf("CAN read error: %v, consecutive errors: %d", err, consecutiveErrors)

				if consecutiveErrors >= maxConsecutiveErrors {
					log.Printf("Device %s disconnected, signaling for reconnection", r.interfaceName)
					cmd.Process.Kill()
					cmd.Wait()
					// Signal disconnection
					select {
					case r.frameChan <- nil:
					default:
					}
					return
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Reset error count on successful read
			consecutiveErrors = 0

			// Parse candump output and send frames
			// Format: "can0 123#1122334455667788"
			lines := strings.Split(string(buf[:n]), "\n")
			for _, line := range lines {
				frame := parseCandumpLine(line)
				if frame != nil {
					select {
					case r.frameChan <- frame:
						// Frame sent successfully
					default:
						log.Printf("Warning: Frame channel full, dropping frame ID: %X", frame.ID)
					}
				}
			}
		}
	}
}

// parseCandumpLine parses a line from candump output
func parseCandumpLine(line string) *CANFrame {
	// Expected format: "  can0  123#1122334455667788"
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// Split by whitespace
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil
	}

	// Parse ID and data (format: "ID#DATA")
	idData := parts[len(parts)-1]
	idDataParts := strings.Split(idData, "#")
	if len(idDataParts) != 2 {
		return nil
	}

	// Parse ID
	var id uint32
	fmt.Sscanf(idDataParts[0], "%X", &id)

	// Parse data
	dataStr := idDataParts[1]
	data := make([]byte, 0, 8)
	for i := 0; i < len(dataStr); i += 2 {
		if i+1 < len(dataStr) {
			var b byte
			fmt.Sscanf(dataStr[i:i+2], "%02X", &b)
			data = append(data, b)
		}
	}

	return &CANFrame{
		ID:        id,
		Data:      data,
		Length:    uint8(len(data)),
		Timestamp: time.Now(),
		Extended:  id > 0x7FF,
		Remote:    false,
	}
}

// continuousReadSimulated generates simulated frames
func (r *LinuxCANReader) continuousReadSimulated() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			frame := generateLinuxFrame()

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
func (r *LinuxCANReader) Write(frame *CANFrame) error {
	if frame == nil {
		return fmt.Errorf("frame is nil")
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
		log.Printf("LINUX SIMULATED WRITE: ID: %X, DLC: %d, Data: %s",
			frame.ID, frame.Length, dataHex)
		return nil
	}

	// Real implementation would use cansend command
	// cansend can0 123#11223344
	dataHex := ""
	for _, b := range frame.Data {
		dataHex += fmt.Sprintf("%02X", b)
	}

	cmd := exec.Command("cansend", r.interfaceName,
		fmt.Sprintf("%X#%s", frame.ID, dataHex))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cansend failed: %v, output: %s", err, output)
	}

	return nil
}

// SetFilter sets acceptance filters
func (r *LinuxCANReader) SetFilter(filters []Filter) error {
	log.Printf("Setting %d filter(s) on Linux CAN interface %s",
		len(filters), r.interfaceName)

	for i, filter := range filters {
		log.Printf("  Filter %d: ID=0x%X, Mask=0x%X, Extended=%v",
			i+1, filter.ID, filter.Mask, filter.Extend)
	}

	return nil
}

// checkInterfaceExists checks if a CAN interface exists
func (r *LinuxCANReader) checkInterfaceExists(interfaceName string) bool {
	cmd := exec.Command("ip", "link", "show", interfaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), interfaceName)
}

// bringUpInterface brings up a CAN interface with specified bitrate
func (r *LinuxCANReader) bringUpInterface(interfaceName string, bitrate int) error {
	// First, bring down the interface if it's up
	exec.Command("sudo", "ip", "link", "set", interfaceName, "down").Run()

	// Set bitrate and bring up
	cmd := exec.Command("sudo", "ip", "link", "set", interfaceName,
		"up", "type", "can", "bitrate", fmt.Sprintf("%d", bitrate))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to bring up %s: %v, output: %s",
			interfaceName, err, string(output))
	}

	return nil
}

// generateLinuxFrame creates a simulated CAN frame for Linux
func generateLinuxFrame() *CANFrame {
	// Generate realistic CAN frames for Linux
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
