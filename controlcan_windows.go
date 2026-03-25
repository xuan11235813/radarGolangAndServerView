//go:build windows

package main

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Constants from ControlCAN.h
const (
	VCI_USBCAN1          = 3
	VCI_USBCAN2          = 4
	VCI_USBCAN2A         = 4
	VCI_USBCAN_E_U       = 20
	VCI_USBCAN_2E_U      = 21
	VCI_CANETTCP         = 22
	VCI_CANETUDP         = 23
	VCI_USBCAN_4E_U      = 24
	VCI_USBCAN_8E_U      = 25
	VCI_PCI9840          = 26
	VCI_PCI5010U         = 27
	VCI_USBCAN_ECAN_U    = 28
	VCI_PCI5020U         = 29
	VCI_USBCAN_2E_U_P    = 31
	VCI_USBCAN_4E_U_P    = 32
	VCI_CANDTU_200UR     = 33
	VCI_CANDTU_200UFR    = 34
	VCI_CANDTU_200FD     = 35
	VCI_CANDTU_800UR     = 36
	VCI_CANDTU_800UFR    = 37
	VCI_CANDTU_800FD     = 38
	VCI_CANDTU_400UR     = 39
	VCI_CANDTU_400UFR    = 40
	VCI_CANDTU_400FD     = 41
	VCI_CANDTU_100UR     = 42
	VCI_CANDTU_100UFR    = 43
	VCI_CANDTU_100FD     = 44
	VCI_CANFDCOM_100U    = 45
	VCI_CANFDCOM_200U    = 46
	VCI_CANFDCOM_400U    = 47
	VCI_CANFDCOM_800U    = 48
	VCI_CANFDCOM_100UC   = 49
	VCI_CANFDCOM_200UC   = 50
	VCI_CANFDCOM_400UC   = 51
	VCI_CANFDCOM_800UC   = 52
	VCI_CANETTCP_GS      = 53
	VCI_CANETUDP_GS      = 54
	VCI_USBCANFD_200U    = 55
	VCI_USBCANFD_100U    = 56
	VCI_USBCANFD_MINI    = 57
	VCI_CANFDCOM_800U_P  = 58
	VCI_CANFDCOM_400U_P  = 59
	VCI_CANFDCOM_200U_P  = 60
	VCI_CANFDCOM_100U_P  = 61
	VCI_CANFDCOM_800UC_P = 62
	VCI_CANFDCOM_400UC_P = 63
	VCI_CANFDCOM_200UC_P = 64
	VCI_CANFDCOM_100UC_P = 65
	VCI_CANETTCP_M       = 66
	VCI_CANETUDP_M       = 67
	VCI_CANETTCP_G       = 68
	VCI_CANETUDP_G       = 69
	VCI_CANETTCP_S       = 70
	VCI_CANETUDP_S       = 71
	VCI_CANETTCP_SE      = 72
	VCI_CANETUDP_SE      = 73
	VCI_CANETTCP_A       = 74
	VCI_CANETUDP_A       = 75
	VCI_CANETTCP_AE      = 76
	VCI_CANETUDP_AE      = 77
	VCI_CANETTCP_PRO     = 78
	VCI_CANETUDP_PRO     = 79
	VCI_CANETTCP_PROW    = 80
	VCI_CANETUDP_PROW    = 81
	VCI_CANETTCP_PROE    = 82
	VCI_CANETUDP_PROE    = 83
	VCI_CANETTCP_PROEW   = 84
	VCI_CANETUDP_PROEW   = 85
)

// VCI_INIT_CONFIG corresponds to VCI_INIT_CONFIG in ControlCAN.h
type VCI_INIT_CONFIG struct {
	AccCode  uint32
	AccMask  uint32
	Reserved uint32
	Filter   uint8
	Timing0  uint8
	Timing1  uint8
	Mode     uint8
}

// VCI_CAN_OBJ corresponds to VCI_CAN_OBJ in ControlCAN.h
type VCI_CAN_OBJ struct {
	ID         uint32
	TimeStamp  uint32
	TimeFlag   uint8
	SendType   uint8
	RemoteFlag uint8
	ExternFlag uint8
	DataLen    uint8
	Data       [8]uint8
	Reserved   [3]uint8
}

// VCI_BOARD_INFO corresponds to VCI_BOARD_INFO in ControlCAN.h
type VCI_BOARD_INFO struct {
	hw_Version     uint16
	fw_Version     uint16
	dr_Version     uint16
	in_Version     uint16
	irq_Num        uint16
	can_Num        uint8
	str_Serial_Num [20]uint8
	str_hw_Type    [40]uint8
	Reserved       [4]uint16
}

// ControlCAN DLL wrapper
type ControlCAN struct {
	dllHandle syscall.Handle

	// Function pointers
	VCI_OpenDevice     uintptr
	VCI_CloseDevice    uintptr
	VCI_InitCAN        uintptr
	VCI_ReadBoardInfo  uintptr
	VCI_ReadErrInfo    uintptr
	VCI_ReadCANStatus  uintptr
	VCI_GetReference   uintptr
	VCI_SetReference   uintptr
	VCI_GetReceiveNum  uintptr
	VCI_ClearBuffer    uintptr
	VCI_StartCAN       uintptr
	VCI_ResetCAN       uintptr
	VCI_Transmit       uintptr
	VCI_Receive        uintptr
	VCI_GetReference2  uintptr
	VCI_SetReference2  uintptr
	VCI_ResumeConfig   uintptr
	VCI_ConnectDevice  uintptr
	VCI_UsbDeviceReset uintptr
}

// NewControlCAN creates a new ControlCAN wrapper
func NewControlCAN(dllPath string) (*ControlCAN, error) {
	dll, err := syscall.LoadLibrary(dllPath)
	if err != nil {
		// Provide more helpful error message for common issues
		errMsg := err.Error()
		if strings.Contains(errMsg, "not a valid Win32 application") {
			return nil, fmt.Errorf("failed to load ControlCAN.dll: architecture mismatch (likely 32-bit DLL with 64-bit executable or vice versa). Try building with GOARCH=386 for 32-bit DLLs")
		}
		return nil, fmt.Errorf("failed to load ControlCAN.dll: %w", err)
	}

	cc := &ControlCAN{dllHandle: dll}

	// Get function addresses
	cc.VCI_OpenDevice = getProcAddress(dll, "VCI_OpenDevice")
	cc.VCI_CloseDevice = getProcAddress(dll, "VCI_CloseDevice")
	cc.VCI_InitCAN = getProcAddress(dll, "VCI_InitCAN")
	cc.VCI_ReadBoardInfo = getProcAddress(dll, "VCI_ReadBoardInfo")
	cc.VCI_ReadErrInfo = getProcAddress(dll, "VCI_ReadErrInfo")
	cc.VCI_ReadCANStatus = getProcAddress(dll, "VCI_ReadCANStatus")
	cc.VCI_GetReference = getProcAddress(dll, "VCI_GetReference")
	cc.VCI_SetReference = getProcAddress(dll, "VCI_SetReference")
	cc.VCI_GetReceiveNum = getProcAddress(dll, "VCI_GetReceiveNum")
	cc.VCI_ClearBuffer = getProcAddress(dll, "VCI_ClearBuffer")
	cc.VCI_StartCAN = getProcAddress(dll, "VCI_StartCAN")
	cc.VCI_ResetCAN = getProcAddress(dll, "VCI_ResetCAN")
	cc.VCI_Transmit = getProcAddress(dll, "VCI_Transmit")
	cc.VCI_Receive = getProcAddress(dll, "VCI_Receive")
	cc.VCI_GetReference2 = getProcAddress(dll, "VCI_GetReference2")
	cc.VCI_SetReference2 = getProcAddress(dll, "VCI_SetReference2")
	cc.VCI_ResumeConfig = getProcAddress(dll, "VCI_ResumeConfig")
	cc.VCI_ConnectDevice = getProcAddress(dll, "VCI_ConnectDevice")
	cc.VCI_UsbDeviceReset = getProcAddress(dll, "VCI_UsbDeviceReset")

	return cc, nil
}

// Close releases the DLL
func (cc *ControlCAN) Close() error {
	return syscall.FreeLibrary(cc.dllHandle)
}

// Helper function to get procedure address
func getProcAddress(dll syscall.Handle, name string) uintptr {
	addr, err := syscall.GetProcAddress(dll, name)
	if err != nil {
		// For this example, we'll panic if function not found
		// In production, you might want to handle this differently
		panic(fmt.Sprintf("failed to get procedure address for %s: %v", name, err))
	}
	return addr
}

// Wrapper functions

func (cc *ControlCAN) OpenDevice(devType, devIndex uint32) uint32 {
	ret, _, _ := syscall.Syscall(cc.VCI_OpenDevice, 2,
		uintptr(devType),
		uintptr(devIndex),
		0)
	return uint32(ret)
}

func (cc *ControlCAN) CloseDevice(devType, devIndex uint32) uint32 {
	ret, _, _ := syscall.Syscall(cc.VCI_CloseDevice, 2,
		uintptr(devType),
		uintptr(devIndex),
		0)
	return uint32(ret)
}

func (cc *ControlCAN) InitCAN(devType, devIndex, canIndex uint32, initConfig *VCI_INIT_CONFIG) uint32 {
	ret, _, _ := syscall.Syscall6(cc.VCI_InitCAN, 4,
		uintptr(devType),
		uintptr(devIndex),
		uintptr(canIndex),
		uintptr(unsafe.Pointer(initConfig)),
		0, 0)
	return uint32(ret)
}

func (cc *ControlCAN) StartCAN(devType, devIndex, canIndex uint32) uint32 {
	ret, _, _ := syscall.Syscall(cc.VCI_StartCAN, 3,
		uintptr(devType),
		uintptr(devIndex),
		uintptr(canIndex))
	return uint32(ret)
}

func (cc *ControlCAN) Transmit(devType, devIndex, canIndex uint32, canObj *VCI_CAN_OBJ, length uint32) uint32 {
	ret, _, _ := syscall.Syscall6(cc.VCI_Transmit, 5,
		uintptr(devType),
		uintptr(devIndex),
		uintptr(canIndex),
		uintptr(unsafe.Pointer(canObj)),
		uintptr(length),
		0)
	return uint32(ret)
}

func (cc *ControlCAN) Receive(devType, devIndex, canIndex uint32, canObj *VCI_CAN_OBJ, length, waitTime uint32) uint32 {
	ret, _, _ := syscall.Syscall9(cc.VCI_Receive, 6,
		uintptr(devType),
		uintptr(devIndex),
		uintptr(canIndex),
		uintptr(unsafe.Pointer(canObj)),
		uintptr(length),
		uintptr(waitTime),
		0, 0, 0)
	return uint32(ret)
}

func (cc *ControlCAN) GetReceiveNum(devType, devIndex, canIndex uint32) uint32 {
	ret, _, _ := syscall.Syscall(cc.VCI_GetReceiveNum, 3,
		uintptr(devType),
		uintptr(devIndex),
		uintptr(canIndex))
	return uint32(ret)
}

func (cc *ControlCAN) ClearBuffer(devType, devIndex, canIndex uint32) uint32 {
	ret, _, _ := syscall.Syscall(cc.VCI_ClearBuffer, 3,
		uintptr(devType),
		uintptr(devIndex),
		uintptr(canIndex))
	return uint32(ret)
}

func (cc *ControlCAN) ResetCAN(devType, devIndex, canIndex uint32) uint32 {
	ret, _, _ := syscall.Syscall(cc.VCI_ResetCAN, 3,
		uintptr(devType),
		uintptr(devIndex),
		uintptr(canIndex))
	return uint32(ret)
}

// Convert VCI_CAN_OBJ to CANFrame
func (obj *VCI_CAN_OBJ) ToCANFrame() *CANFrame {
	data := make([]byte, obj.DataLen)
	for i := 0; i < int(obj.DataLen); i++ {
		data[i] = obj.Data[i]
	}

	return &CANFrame{
		ID:        obj.ID,
		Data:      data,
		Length:    obj.DataLen,
		Timestamp: time.Now(),
		Extended:  obj.ExternFlag != 0,
		Remote:    obj.RemoteFlag != 0,
	}
}

// Convert CANFrame to VCI_CAN_OBJ
func (frame *CANFrame) ToVCI_CAN_OBJ() *VCI_CAN_OBJ {
	obj := &VCI_CAN_OBJ{
		ID:         frame.ID,
		SendType:   1, // Single shot transmission
		RemoteFlag: boolToUint8(frame.Remote),
		ExternFlag: boolToUint8(frame.Extended),
		DataLen:    frame.Length,
	}

	// Copy data
	for i := 0; i < int(frame.Length); i++ {
		if i < len(frame.Data) {
			obj.Data[i] = frame.Data[i]
		}
	}

	return obj
}

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
