package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
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
	ID         uint8
	DistLong   float64
	DistLat    float64
	VrelLong   float64
	VrelLat    float64
	DynProp    uint8
	RCS        float64
	Timestamp  int64
	DeviceName string
}

type RecordClustering struct {
	ID         uint8
	DistLong   float64
	DistLat    float64
	VrelLong   float64
	VrelLat    float64
	DynProp    uint8
	RCS        float64
	Timestamp  uint64
	DeviceName string
}

// GetCurrentTimestampMillis returns the current UTC timestamp in milliseconds
func GetCurrentTimestampMillis() int64 {
	return time.Now().UTC().UnixMilli()
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

type Device struct {
	interfaceNameString string
	Cluster0StatusCh    chan Cluster0Status
	Cluster1GeneralCh   chan Cluster1General
	CSVRecordCh         chan Cluster1General // Channel for CSV recording with large buffer
}

func NewDevice(deviceName string) *Device {
	d := &Device{}
	d.interfaceNameString = deviceName
	d.Cluster0StatusCh = make(chan Cluster0Status, 4)
	d.Cluster1GeneralCh = make(chan Cluster1General, 4)
	d.CSVRecordCh = make(chan Cluster1General, 10000) // Large buffer to prevent data loss
	return d
}

// generateCSVFilename creates a filename in format year_month_day_hour_minute_second_deviceName.csv
func generateCSVFilename(deviceName string) string {
	now := time.Now()
	return fmt.Sprintf("%d_%02d_%02d_%02d_%02d_%02d_%s.csv",
		now.Year(),
		now.Month(),
		now.Day(),
		now.Hour(),
		now.Minute(),
		now.Second(),
		deviceName)
}

// csvWriterGoroutine writes Cluster1General records to CSV file
func csvWriterGoroutine(csvCh <-chan Cluster1General, deviceName string, done <-chan struct{}) {
	filename := generateCSVFilename(deviceName)

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Failed to create CSV file %s: %v", filename, err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header
	header := []string{"Timestamp", "ID", "DistLong", "DistLat", "VrelLong", "VrelLat", "DynProp", "RCS", "DeviceName"}
	if err := writer.Write(header); err != nil {
		log.Printf("Failed to write CSV header: %v", err)
		return
	}

	recordCount := 0
	for {
		select {
		case <-done:
			log.Printf("CSV writer for device %s stopping. Wrote %d records to %s", deviceName, recordCount, filename)
			return
		case record, ok := <-csvCh:
			if !ok {
				log.Printf("CSV channel closed for device %s. Wrote %d records to %s", deviceName, recordCount, filename)
				return
			}

			row := []string{
				strconv.FormatInt(record.Timestamp, 10),
				strconv.Itoa(int(record.ID)),
				strconv.FormatFloat(record.DistLong, 'f', 4, 64),
				strconv.FormatFloat(record.DistLat, 'f', 4, 64),
				strconv.FormatFloat(record.VrelLong, 'f', 4, 64),
				strconv.FormatFloat(record.VrelLat, 'f', 4, 64),
				strconv.Itoa(int(record.DynProp)),
				strconv.FormatFloat(record.RCS, 'f', 4, 64),
				record.DeviceName,
			}

			if err := writer.Write(row); err != nil {
				log.Printf("Failed to write CSV record: %v", err)
			} else {
				recordCount++
			}

			// Flush periodically to ensure data is written
			if recordCount%100 == 0 {
				writer.Flush()
			}
		}
	}
}

func (d *Device) deviceMainLoop() {
	// Parse command line arguments
	interfaceName := d.interfaceNameString
	bitrate := 500000

	// Create CAN reader based on platform
	reader, err := NewCANReader()
	if err != nil {
		log.Fatalf("Failed to create CAN reader: %v", err)
	}

	// Open the CAN interface
	err = reader.Open(interfaceName, bitrate)
	if err != nil {
		log.Fatalf("Failed to open CAN interface %s: %v", interfaceName, err)
	}
	defer reader.Close()

	fmt.Printf("CAN interface %s opened successfully. Press Ctrl+C to exit.\n", interfaceName)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start continuous reading
	frameChan, err := reader.StartReading()
	if err != nil {
		log.Fatalf("Failed to start continuous reading: %v", err)
	}

	// Create done channel for CSV writer goroutine
	done := make(chan struct{})

	// Start CSV writer goroutine
	go csvWriterGoroutine(d.CSVRecordCh, d.interfaceNameString, done)

	// Main reading loop
	go implementFrameData(d.Cluster0StatusCh, d.Cluster1GeneralCh, d.CSVRecordCh)
	readLoopFromChannel(frameChan, sigChan, d.Cluster0StatusCh, d.Cluster1GeneralCh, d.interfaceNameString)

	// Signal CSV writer to stop
	close(done)
}

// Platform-specific implementations will be in separate files with build tags

func ParseCANFrame(frame CANFrame, ch0 chan Cluster0Status, ch1 chan Cluster1General, deviceName string) {
	var currentTime int64 = GetCurrentTimestampMillis()
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
		currentTime = GetCurrentTimestampMillis()

		ch0 <- status

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
			ID:         uint8(rawID),
			DistLong:   float64(rawDistLong)*0.2 - 500.0,
			DistLat:    float64(rawDistLat)*0.2 - 102.3,
			VrelLong:   float64(rawVrelLong)*0.25 - 128.0,
			VrelLat:    float64(rawVrelLat)*0.25 - 64.0,
			DynProp:    uint8(rawDynProp),
			RCS:        float64(rawRCS)*0.5 - 64.0,
			Timestamp:  currentTime,
			DeviceName: deviceName,
		}

		ch1 <- msg
	default:
		// ignore other IDs
	}
}

// readLoopFromChannel continuously reads CAN frames from channel
func readLoopFromChannel(frameChan <-chan *CANFrame, sigChan chan os.Signal, ch0 chan Cluster0Status, ch1 chan Cluster1General, deviceName string) {
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

			ParseCANFrame(*frame, ch0, ch1, deviceName)
		}
	}
}

// Config represents the configuration file structure
type Config struct {
	Devices []string `json:"devices"`
}

// loadConfig reads the configuration file
func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func main() {
	// Load configuration
	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load config.json: %v", err)
	}

	if len(config.Devices) == 0 {
		log.Fatal("No devices configured in config.json")
	}

	fmt.Printf("Loaded %d devices from config.json: %v\n", len(config.Devices), config.Devices)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Use WaitGroup to wait for all devices to finish
	var wg sync.WaitGroup

	// Start each device in a separate goroutine
	for _, deviceName := range config.Devices {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			device := NewDevice(name)
			device.deviceMainLoop()
		}(deviceName)
	}

	// Wait for shutdown signal
	<-sigChan
	fmt.Printf("\nReceived shutdown signal. Exiting...\n")

	// Wait for all device goroutines to finish
	wg.Wait()
	fmt.Println("All devices stopped.")
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

func implementFrameData(ch0 chan Cluster0Status, ch1 chan Cluster1General, csvCh chan Cluster1General) {
	// Print header once at the beginning
	printClusterHeader()

	for {
		select {
		case msg := <-ch0:
			fmt.Printf("[0x600] %+v\n", msg)

		case msg := <-ch1:
			// Use the compact format for real-time updates
			printCluster1GeneralLine(msg)
			// Send to CSV recording channel (non-blocking to prevent any slowdown)
			select {
			case csvCh <- msg:
				// Successfully sent to CSV channel
			default:
				// Channel full, log warning but don't block
				log.Printf("Warning: CSV recording channel full for device, dropping record")
			}
		}
	}
}
