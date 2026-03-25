package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// CANFrame represents a CAN bus data frame
type CANFrame struct {
	ID        uint32
	Data      []byte
	Length    uint8
	Timestamp time.Time
	Extended  bool
	Remote    bool
}

type Cluster0Status struct {
	NofClustersNear  uint8  // 8 bits
	NofClustersFar   uint8  // 8 bits
	MeasCounter      uint16 // 16 bits
	InterfaceVersion uint8  // 4 bits (stored in uint8)
}

type Cluster1General struct {
	ID       uint8
	DistLong float64
	DistLat  float64
	VrelLong float64
	VrelLat  float64
	DynProp  uint8
	RCS      float64
}

type RecordClustering struct {
}

// CANReader interface defines methods for reading CAN bus data
type CANReader interface {
	// Open opens the CAN bus interface
	Open(interfaceName string, bitrate int) error
	// Close closes the CAN bus interface
	Close() error
	// Read reads a CAN frame with timeout (blocking)
	Read(timeout time.Duration) (*CANFrame, error)
	// StartReading starts continuous non-blocking reading and returns a channel for frames
	StartReading() (<-chan *CANFrame, error)
	// Write writes a CAN frame
	Write(frame *CANFrame) error
	// SetFilter sets acceptance filters (optional)
	SetFilter(filters []Filter) error
}

// Filter represents a CAN acceptance filter
type Filter struct {
	ID     uint32
	Mask   uint32
	Extend bool
}

var Cluster0StatusCh = make(chan Cluster0Status, 4)
var Cluster1GeneralCh = make(chan Cluster1General, 4)

// Platform-specific implementations will be in separate files with build tags

func main() {

	// Parse command line arguments
	interfaceName := flag.String("interface", "can0", "CAN interface name (e.g., can0, vcan0)")
	bitrate := flag.Int("bitrate", 500000, "CAN bus bitrate (e.g., 500000 for 500kbps)")
	showHelp := flag.Bool("help", false, "Show help")

	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	fmt.Printf("CAN Bus Reader - Cross Platform\n")
	fmt.Printf("Interface: %s, Bitrate: %d\n", *interfaceName, *bitrate)

	// Create CAN reader based on platform
	reader, err := NewCANReader()
	if err != nil {
		log.Fatalf("Failed to create CAN reader: %v", err)
	}

	// Open the CAN interface
	err = reader.Open(*interfaceName, *bitrate)
	if err != nil {
		log.Fatalf("Failed to open CAN interface %s: %v", *interfaceName, err)
	}
	defer reader.Close()

	fmt.Println("CAN interface opened successfully. Press Ctrl+C to exit.")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start continuous reading
	frameChan, err := reader.StartReading()
	if err != nil {
		log.Fatalf("Failed to start continuous reading: %v", err)
	}

	// Main reading loop
	go implementFrameData()
	readLoopFromChannel(frameChan, sigChan)

}

// readLoopFromChannel continuously reads CAN frames from channel
func readLoopFromChannel(frameChan <-chan *CANFrame, sigChan chan os.Signal) {
	frameCount := 0
	startTime := time.Now()

	for {
		select {
		case <-sigChan:
			fmt.Printf("\nReceived shutdown signal. Exiting...\n")
			duration := time.Since(startTime)
			fmt.Printf("Processed %d frames in %v (%.2f frames/sec)\n",
				frameCount, duration, float64(frameCount)/duration.Seconds())
			return
		case frame, ok := <-frameChan:
			if !ok {
				log.Printf("Frame channel closed, exiting")
				return
			}

			if frame == nil {
				continue
			}

			frameCount++
			//printFrame(frame, frameCount)

			ParseCANFrame(*frame)
		}
	}
}

// readLoop continuously reads CAN frames (legacy blocking version)
func readLoop(reader CANReader, sigChan chan os.Signal) {
	frameCount := 0
	startTime := time.Now()

	for {
		select {
		case <-sigChan:
			fmt.Printf("\nReceived shutdown signal. Exiting...\n")
			duration := time.Since(startTime)
			fmt.Printf("Processed %d frames in %v (%.2f frames/sec)\n",
				frameCount, duration, float64(frameCount)/duration.Seconds())
			return
		default:
			// Read with 100ms timeout
			frame, err := reader.Read(100 * time.Millisecond)
			if err != nil {
				// Check if it's a timeout error (expected)
				if err.Error() != "timeout" {
					log.Printf("Read error: %v", err)
				}
				continue
			}

			frameCount++
			//printFrame(frame, frameCount)

			ParseCANFrame(*frame)
		}
	}
}

// printFrame prints CAN frame information
func printFrame(frame *CANFrame, count int) {
	if frame == nil {
		return
	}

	// Format data bytes as hex
	dataHex := ""
	for i, b := range frame.Data {
		if i > 0 {
			dataHex += " "
		}
		dataHex += fmt.Sprintf("%02X", b)
	}

	// Determine frame type
	frameType := "DATA"
	if frame.Remote {
		frameType = "RTR"
	}

	// Format ID with appropriate notation for extended frames
	idFormat := "%03X"
	if frame.Extended {
		idFormat = "%08X"
	}

	fmt.Printf("[%6d] %s ID: "+idFormat+" DLC: %d Data: %s\n",
		count, frameType, frame.ID, frame.Length, dataHex)
}

// NewCANReader creates a platform-specific CAN reader
func NewCANReader() (CANReader, error) {
	return newPlatformCANReader()
}

func extractBits(data []byte, startBit, length int) uint64 {
	var result uint64 = 0
	startBit = startBit - length + 2
	for i := 0; i < length; i++ {
		bitPos := startBit + i
		byteIndex := bitPos / 8
		bitIndex := 7 - bitPos%8

		bit := (data[byteIndex] >> bitIndex) & 1
		result = result<<1 + uint64(bit)
	}
	return result
}

func extractBitsMotorola(data []byte, startBit, length int) uint64 {
	var result uint64 = 0
	currentBit := startBit
	byteIndex := startBit / 8
	realLowerstPos := (byteIndex+1)*8 - currentBit + byteIndex*8 - 1

	for i := 0; i < length; i++ {
		realCurrentPos := realLowerstPos - i
		byteIndex := realCurrentPos / 8
		bitIndex := 7 - realCurrentPos%8

		bit := (data[byteIndex] >> (bitIndex)) & 1
		result = result + (uint64(bit) << i)
	}
	return result
}

func ParseCANFrame(frame CANFrame) {
	switch frame.ID {

	// =========================
	// 0x600 (already implemented)
	// =========================
	case 0x600:
		if len(frame.Data) < 5 {
			fmt.Println("Invalid data length")
			return
		}

		data := frame.Data

		status := Cluster0Status{
			NofClustersNear:  data[0],
			NofClustersFar:   data[1],
			MeasCounter:      uint16(data[3]) | uint16(data[2])<<8,
			InterfaceVersion: (data[4] >> 4) & 0x0F,
		}

		Cluster0StatusCh <- status

	// =========================
	// 0x701 (NEW)
	// =========================
	case 0x701:
		if len(frame.Data) < 8 {
			fmt.Println("Invalid data length")
			return
		}

		data := frame.Data

		rawID := extractBitsMotorola(data, 0, 8)
		rawDistLong := extractBitsMotorola(data, 19, 13)
		rawDistLat := extractBitsMotorola(data, 24, 10)
		rawVrelLong := extractBitsMotorola(data, 46, 10)
		rawDynProp := extractBitsMotorola(data, 48, 3)
		rawVrelLat := extractBitsMotorola(data, 53, 9)
		rawRCS := extractBitsMotorola(data, 56, 8)

		msg := Cluster1General{
			ID:       uint8(rawID),
			DistLong: float64(rawDistLong)*0.2 - 500.0,
			DistLat:  float64(rawDistLat)*0.2 - 102.3,
			VrelLong: float64(rawVrelLong)*0.25 - 128.0,
			VrelLat:  float64(rawVrelLat)*0.25 - 64.0,
			DynProp:  uint8(rawDynProp),
			RCS:      float64(rawRCS)*0.5 - 64.0,
		}

		Cluster1GeneralCh <- msg
	default:
		// ignore other IDs
	}
}

func implementFrameData() {
	// Print header once at the beginning
	printClusterHeader()

	for {
		select {
		case msg := <-Cluster0StatusCh:
			fmt.Printf("[0x600] %+v\n", msg)

		case msg := <-Cluster1GeneralCh:
			// Use the compact format for real-time updates
			printCluster1GeneralLine(msg)
		}
	}
}
