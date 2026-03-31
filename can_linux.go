//go:build linux

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.einride.tech/can"
	"go.einride.tech/can/pkg/socketcan"
)

// LinuxCANReader implements CANReader for Linux using SocketCAN
type LinuxCANReader struct {
	interfaceName string
	simulated     bool
	connected     bool

	// For continuous reading
	frameChan    chan *CANFrame
	stopChan     chan struct{}
	reading      bool
	readingMutex sync.Mutex

	// SocketCAN connection
	conn        net.Conn
	receiver    *socketcan.Receiver
	transmitter *socketcan.Transmitter
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

	retryInterval := 2 * time.Second

	for {
		// Check if CAN interface exists
		if !r.checkInterfaceExists(interfaceName) {
			log.Printf("CAN interface %s not found, retrying in %v...", interfaceName, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		// Check if interface is up, if not bring it up
		if !r.isInterfaceUp(interfaceName) {
			log.Printf("CAN interface %s is down, attempting to bring it up", interfaceName)
			err := r.bringUpInterface(interfaceName, bitrate)
			if err != nil {
				log.Printf("Failed to bring up %s: %v, retrying in %v...", interfaceName, err, retryInterval)
				time.Sleep(retryInterval)
				continue
			}
			log.Printf("CAN interface %s brought up successfully", interfaceName)
		}

		// Try to dial SocketCAN connection
		ctx := context.Background()
		conn, err := socketcan.DialContext(ctx, "can", interfaceName)
		if err != nil {
			log.Printf("Failed to open SocketCAN interface %s: %v, retrying in %v...", interfaceName, err, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		r.conn = conn
		r.receiver = socketcan.NewReceiver(conn)
		r.transmitter = socketcan.NewTransmitter(conn)
		r.simulated = false
		r.connected = true

		log.Printf("CAN interface %s opened successfully via SocketCAN", interfaceName)
		return nil
	}
}

// Close closes the CAN interface
func (r *LinuxCANReader) Close() error {
	r.StopReading()
	r.connected = false
	if r.receiver != nil {
		r.receiver.Close()
		r.receiver = nil
	}
	if r.transmitter != nil {
		r.transmitter.Close()
		r.transmitter = nil
	}
	if r.conn != nil {
		err := r.conn.Close()
		r.conn = nil
		log.Printf("Closing Linux CAN interface %s", r.interfaceName)
		return err
	}
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

	// Real SocketCAN reading
	if r.receiver == nil {
		return nil, fmt.Errorf("connection not established")
	}

	// Use a goroutine with timeout for reading
	frameChan := make(chan *can.Frame, 1)
	errChan := make(chan error, 1)

	go func() {
		if r.receiver.Receive() {
			frame := r.receiver.Frame()
			frameChan <- &frame
		} else {
			errChan <- r.receiver.Err()
		}
	}()

	select {
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	case err := <-errChan:
		return nil, err
	case frame := <-frameChan:
		data := make([]byte, frame.Length)
		copy(data, frame.Data[:frame.Length])
		return &CANFrame{
			ID:        frame.ID,
			Data:      data,
			Length:    frame.Length,
			Timestamp: time.Now(),
			Extended:  frame.IsExtended,
			Remote:    frame.IsRemote,
		}, nil
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

	log.Printf("Starting real SocketCAN reading on interface %s", r.interfaceName)

	if r.receiver == nil {
		log.Printf("Error: SocketCAN receiver is nil")
		return
	}

	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	for {
		select {
		case <-r.stopChan:
			log.Printf("Stop signal received, stopping SocketCAN reader")
			return
		default:
			if r.receiver.Receive() {
				// Reset error count on successful read
				consecutiveErrors = 0

				frame := r.receiver.Frame()

				// Check if it's an error frame
				if r.receiver.HasErrorFrame() {
					log.Printf("Received error frame: %v", r.receiver.ErrorFrame())
					continue
				}

				// Convert can.Frame to CANFrame
				data := make([]byte, frame.Length)
				copy(data, frame.Data[:frame.Length])

				canFrame := &CANFrame{
					ID:        frame.ID,
					Data:      data,
					Length:    frame.Length,
					Timestamp: time.Now(),
					Extended:  frame.IsExtended,
					Remote:    frame.IsRemote,
				}

				// Send frame to channel (non-blocking)
				select {
				case r.frameChan <- canFrame:
					// Frame sent successfully
				default:
					log.Printf("Warning: Frame channel full, dropping frame ID: %X", canFrame.ID)
				}
			} else {
				// Receive returned false, check for error
				err := r.receiver.Err()
				if err != nil {
					consecutiveErrors++
					log.Printf("CAN read error: %v, consecutive errors: %d", err, consecutiveErrors)

					if consecutiveErrors >= maxConsecutiveErrors {
						log.Printf("Device %s disconnected, signaling for reconnection", r.interfaceName)
						// Signal disconnection
						select {
						case r.frameChan <- nil:
						default:
						}
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
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

	if r.transmitter == nil {
		return fmt.Errorf("connection not established")
	}

	// Create can.Frame
	var canFrame can.Frame
	canFrame.ID = frame.ID
	canFrame.Length = frame.Length
	canFrame.IsExtended = frame.Extended
	canFrame.IsRemote = frame.Remote
	copy(canFrame.Data[:], frame.Data)

	ctx := context.Background()
	return r.transmitter.TransmitFrame(ctx, canFrame)
}

// SetFilter sets acceptance filters
func (r *LinuxCANReader) SetFilter(filters []Filter) error {
	log.Printf("Setting %d filter(s) on Linux CAN interface %s",
		len(filters), r.interfaceName)

	for i, filter := range filters {
		log.Printf("  Filter %d: ID=0x%X, Mask=0x%X, Extended=%v",
			i+1, filter.ID, filter.Mask, filter.Extend)
	}

	// Note: SocketCAN filtering would require ioctl calls
	// For now, we'll filter in software
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

// isInterfaceUp checks if a CAN interface is already up and running
func (r *LinuxCANReader) isInterfaceUp(interfaceName string) bool {
	cmd := exec.Command("ip", "link", "show", interfaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	// Check if the interface state is "UP"
	outputStr := string(output)
	return strings.Contains(outputStr, "state UP") || strings.Contains(outputStr, "UNKNOWN")
}

// bringUpInterface brings up a CAN interface with specified bitrate
func (r *LinuxCANReader) bringUpInterface(interfaceName string, bitrate int) error {
	// First, bring down the interface if it's up (requires sudo)
	exec.Command("sudo", "ip", "link", "set", interfaceName, "down").Run()

	// Set bitrate (requires sudo)
	cmd := exec.Command("sudo", "ip", "link", "set", interfaceName, "type", "can", "bitrate", fmt.Sprintf("%d", bitrate))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set bitrate: %v, output: %s", err, string(output))
	}

	// Bring up the interface (requires sudo)
	cmd = exec.Command("sudo", "ip", "link", "set", interfaceName, "up")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to bring up: %v, output: %s", err, string(output))
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
