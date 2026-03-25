//go:build !linux && !windows

package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// StubCANReader implements CANReader for unsupported platforms
type StubCANReader struct {
	simulated bool

	// For continuous reading
	frameChan    chan *CANFrame
	stopChan     chan struct{}
	reading      bool
	readingMutex sync.Mutex
}

// newPlatformCANReader creates a stub CAN reader
func newPlatformCANReader() (CANReader, error) {
	return &StubCANReader{
		simulated: true,
		frameChan: make(chan *CANFrame, 1000),
		stopChan:  make(chan struct{}),
	}, nil
}

// Open opens a CAN interface (stub)
func (r *StubCANReader) Open(interfaceName string, bitrate int) error {
	log.Printf("CAN interface not supported on this platform")
	log.Printf("Interface: %s, Bitrate: %d", interfaceName, bitrate)
	log.Printf("Running in simulation mode")
	r.simulated = true
	return nil
}

// Close closes the CAN interface
func (r *StubCANReader) Close() error {
	r.StopReading()
	log.Printf("Closing stub CAN interface")
	return nil
}

// Read reads a CAN frame with timeout (blocking, for backward compatibility)
func (r *StubCANReader) Read(timeout time.Duration) (*CANFrame, error) {
	if !r.simulated {
		return nil, fmt.Errorf("CAN not supported on this platform")
	}

	// Simulate reading with timeout
	select {
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	case <-time.After(100 * time.Millisecond):
		// Generate a simulated CAN frame
		return generateStubFrame(), nil
	}
}

// StartReading starts continuous non-blocking reading and returns a channel for frames
func (r *StubCANReader) StartReading() (<-chan *CANFrame, error) {
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
func (r *StubCANReader) StopReading() {
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
func (r *StubCANReader) continuousRead() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			frame := generateStubFrame()

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

// Write writes a CAN frame (simulated)
func (r *StubCANReader) Write(frame *CANFrame) error {
	if frame == nil {
		return fmt.Errorf("frame is nil")
	}

	if !r.simulated {
		return fmt.Errorf("CAN not supported on this platform")
	}

	// Log the frame that would be sent
	dataHex := ""
	for i, b := range frame.Data {
		if i > 0 {
			dataHex += " "
		}
		dataHex += fmt.Sprintf("%02X", b)
	}

	log.Printf("STUB WRITE: ID: %X, DLC: %d, Data: %s", frame.ID, frame.Length, dataHex)
	return nil
}

// SetFilter sets acceptance filters
func (r *StubCANReader) SetFilter(filters []Filter) error {
	log.Printf("Setting %d filter(s) on stub CAN interface", len(filters))
	return nil
}

// generateStubFrame creates a test CAN frame for simulation
func generateStubFrame() *CANFrame {
	// Simple counter to generate different frames
	counter := time.Now().Unix() % 10

	return &CANFrame{
		ID:        uint32(0x100 + counter),
		Data:      []byte{byte(counter), byte(counter + 1), byte(counter + 2)},
		Length:    3,
		Timestamp: time.Now(),
		Extended:  false,
		Remote:    false,
	}
}
