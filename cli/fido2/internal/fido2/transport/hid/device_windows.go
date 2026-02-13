//go:build windows
// +build windows

// Package hid provides a platform-independent interface for accessing USB HID devices.
// This file contains the Windows specific implementation using syscalls to setupapi, hid, and kernel32 DLLs.
package hid

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"syscall"
	"unsafe"
)

type overlapped struct {
	n   uint32
	ovl struct {
		internal     uintptr
		internalHigh uintptr
		offset       uint32
		offsetHigh   uint32
		hEvent       uintptr
	}
}

type _GUID struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]uint8
}

type _SP_DEVICE_INTERFACE_DATA struct {
	cbSize   uint32
	guid     _GUID
	flags    uint32
	reserved uintptr
}

type _SP_DEVICE_INTERFACE_DETAIL_DATA_W struct {
	cbSize     uint32
	devicePath [1]uint16
}

type _HIDD_ATTRIBUTES struct {
	size      uint32
	vendorID  uint16
	productID uint16
	version   uint16
}

type _HIDP_CAPS struct {
	usage                     uint16
	usagePage                 uint16
	inputReportByteLength     uint16
	outputReportByteLength    uint16
	featureReportByteLength   uint16
	reserved                  [17]uint16
	numberLinkCollectionNodes uint16
	numberInputButtonCaps     uint16
	numberInputValueCaps      uint16
	numberInputDataIndices    uint16
	numberOutputButtonCaps    uint16
	numberOutputValueCaps     uint16
	numberOutputDataIndices   uint16
	numberFeatureButtonCaps   uint16
	numberFeatureValueCaps    uint16
	numberFeatureDataIndices  uint16
}

type deviceExtra struct {
	file      uintptr
	flock     uintptr
	flockPath string
}

const (
	null uintptr = 0

	_GENERIC_WRITE uint32 = 0x40000000
	_GENERIC_READ  uint32 = 0x80000000

	_OPEN_EXISTING uint32 = 0x00000003

	_FILE_FLAG_OVERLAPPED uint32 = 0x40000000

	_FILE_SHARE_READ  uint32 = 0x00000001
	_FILE_SHARE_WRITE uint32 = 0x00000002

	_LOCKFILE_FAIL_IMMEDIATELY uint32 = 0x00000001
	_LOCKFILE_EXCLUSIVE_LOCK   uint32 = 0x00000002

	_IOCTL_HID_SET_FEATURE uint32 = 0x000b0191
	_IOCTL_HID_GET_FEATURE uint32 = 0x000b0192

	_ERROR_SUCCESS             syscall.Errno = 0x00000000
	_ERROR_LOCK_VIOLATION      syscall.Errno = 0x00000021
	_ERROR_INSUFFICIENT_BUFFER syscall.Errno = 0x0000007a
	_ERROR_IO_PENDING          syscall.Errno = 0x000003e5
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	_CreateEventW        = kernel32.NewProc("CreateEventW")
	_CreateFileW         = kernel32.NewProc("CreateFileW")
	_CloseHandle         = kernel32.NewProc("CloseHandle")
	_DeviceIoControl     = kernel32.NewProc("DeviceIoControl")
	_GetOverlappedResult = kernel32.NewProc("GetOverlappedResult")
	_LockFile            = kernel32.NewProc("LockFile")
	_ReadFile            = kernel32.NewProc("ReadFile")
	_WriteFile           = kernel32.NewProc("WriteFile")
)

const (
	_DIGCF_PRESENT         uint32 = 0x00000002
	_DIGCF_DEVICEINTERFACE uint32 = 0x00000010
)

var (
	setupapi                          = syscall.NewLazyDLL("setupapi.dll")
	_SetupDiDestroyDeviceInfoList     = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	_SetupDiEnumDeviceInterfaces      = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	_SetupDiGetClassDevsW             = setupapi.NewProc("SetupDiGetClassDevsW")
	_SetupDiGetDeviceInterfaceDetailW = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
)

const (
	_HIDP_STATUS_SUCCESS uint32 = 0x00110000
)

var (
	hid                         = syscall.NewLazyDLL("hid.dll")
	_HidD_FreePreparsedData     = hid.NewProc("HidD_FreePreparsedData")
	_HidD_GetAttributes         = hid.NewProc("HidD_GetAttributes")
	_HidD_GetHidGuid            = hid.NewProc("HidD_GetHidGuid")
	_HidD_GetManufacturerString = hid.NewProc("HidD_GetManufacturerString")
	_HidD_GetPreparsedData      = hid.NewProc("HidD_GetPreparsedData")
	_HidD_GetProductString      = hid.NewProc("HidD_GetProductString")
	_HidD_GetSerialNumberString = hid.NewProc("HidD_GetSerialNumberString")
	_HidP_GetCaps               = hid.NewProc("HidP_GetCaps")
)

// call executes a syscall with the given arguments. It handles overlapped I/O if an overlapped structure is passed.
func call(p *syscall.LazyProc, args ...any) (uintptr, error) {
	var ovl *overlapped

	v := make([]uintptr, len(args))
	for i, arg := range args {
		switch arg := arg.(type) {
		case *overlapped:
			ev, _, err := _CreateEventW.Call(null, 0, 0, null)
			if err != nil {
				errno, match := err.(syscall.Errno)
				if !match {
					return 0, err
				}
				if errno != _ERROR_SUCCESS {
					return 0, errno
				}
			}
			defer _CloseHandle.Call(ev)
			if arg == nil {
				return 0, errors.New("call: overlapped is nil")
			}
			ovl = arg
			ovl.ovl.hEvent = ev
			v[i] = uintptr(unsafe.Pointer(&ovl.ovl))
		case uint8:
			v[i] = uintptr(arg)
		case uint16:
			v[i] = uintptr(arg)
		case uint32:
			v[i] = uintptr(arg)
		case uint64:
			v[i] = uintptr(arg)
		case uint:
			v[i] = uintptr(arg)
		case int8:
			v[i] = uintptr(arg)
		case int16:
			v[i] = uintptr(arg)
		case int32:
			v[i] = uintptr(arg)
		case int64:
			v[i] = uintptr(arg)
		case int:
			v[i] = uintptr(arg)
		case unsafe.Pointer:
			v[i] = uintptr(arg)
		case uintptr:
			v[i] = arg
		default:
			return 0, fmt.Errorf("call: argument is not supported: %s", arg)
		}
	}

	r, _, err := p.Call(v...)
	if err != nil {
		errno, match := err.(syscall.Errno)
		if !match {
			return 0, err
		}
		if errno != _ERROR_SUCCESS && (ovl == nil || errno != _ERROR_IO_PENDING) {
			return 0, errno
		}
	}

	if ovl != nil {
		// the functions that accept OVERLAPPED always have a file handle as first argument
		if _, _, err = _GetOverlappedResult.Call(v[0], uintptr(unsafe.Pointer(&ovl.ovl)), uintptr(unsafe.Pointer(&ovl.n)), 1); err != nil {
			errno, match := err.(syscall.Errno)
			if !match {
				return 0, err
			}
			if errno != _ERROR_SUCCESS {
				return 0, errno
			}
		}
	}

	return r, nil
}

// ioctl performs a DeviceIoControl call.
func ioctl(fd uintptr, req uint32, in []byte, out []byte) (int, error) {
	var (
		inb  unsafe.Pointer
		outb unsafe.Pointer
		inl  uint32
		outl uint32
		rv   uint32
	)
	if in != nil {
		inb = unsafe.Pointer(&in[0])
		inl = uint32(len(in))
	}
	if out != nil {
		outb = unsafe.Pointer(&out[0])
		outl = uint32(len(out))
	}

	ovl := overlapped{}
	if _, err := call(_DeviceIoControl, fd, req, inb, inl, outb, outl, unsafe.Pointer(&rv), &ovl); err != nil {
		return 0, err
	}
	return int(ovl.n), nil
}

func enumerate() ([]*Device, error) {
	guid := _GUID{}
	if _, err := call(_HidD_GetHidGuid, unsafe.Pointer(&guid)); err != nil {
		return nil, err
	}

	devInfo, err := call(_SetupDiGetClassDevsW, unsafe.Pointer(&guid), null, null, _DIGCF_PRESENT|_DIGCF_DEVICEINTERFACE)
	if err != nil {
		return nil, err
	}
	defer call(_SetupDiDestroyDeviceInfoList, devInfo)

	idx := uint32(0)
	rv := []*Device{}

	for {
		itf := _SP_DEVICE_INTERFACE_DATA{}
		itf.cbSize = uint32(unsafe.Sizeof(itf))

		b, err := call(_SetupDiEnumDeviceInterfaces, devInfo, null, unsafe.Pointer(&guid), idx, unsafe.Pointer(&itf))
		idx++
		if b == 0 {
			break
		}
		if err != nil {
			continue
		}

		reqSize := uintptr(0)
		if _, err := call(_SetupDiGetDeviceInterfaceDetailW, devInfo, unsafe.Pointer(&itf), null, 0, unsafe.Pointer(&reqSize), null); err != nil {
			if errno, match := err.(syscall.Errno); match && errno != _ERROR_INSUFFICIENT_BUFFER {
				continue
			}
		}

		detailBuf := make([]uint16, reqSize/unsafe.Sizeof(uint16(0)))
		detail := (*_SP_DEVICE_INTERFACE_DETAIL_DATA_W)(unsafe.Pointer(&detailBuf[0]))
		detail.cbSize = uint32(unsafe.Sizeof(_SP_DEVICE_INTERFACE_DETAIL_DATA_W{}))

		if _, err := call(_SetupDiGetDeviceInterfaceDetailW, devInfo, unsafe.Pointer(&itf), unsafe.Pointer(detail), reqSize, null, null); err != nil {
			continue
		}

		pathW := detailBuf[unsafe.Offsetof(detail.devicePath)/unsafe.Sizeof(detailBuf[0]) : len(detailBuf)-1]

		d := func() *Device {
			f, err := call(_CreateFileW, unsafe.Pointer(&pathW[0]), 0, _FILE_SHARE_READ|_FILE_SHARE_WRITE, null, _OPEN_EXISTING, 0, null)
			if err != nil {
				return nil
			}
			defer call(_CloseHandle, f)

			attr := _HIDD_ATTRIBUTES{}
			if _, err := call(_HidD_GetAttributes, f, unsafe.Pointer(&attr)); err != nil {
				return nil
			}

			rv := &Device{
				path:      syscall.UTF16ToString(pathW),
				vendorID:  attr.vendorID,
				productID: attr.productID,
				version:   attr.version,
			}
			buf := make([]uint16, 4092/unsafe.Sizeof(uint16(0)))
			if _, err := call(_HidD_GetManufacturerString, f, unsafe.Pointer(&buf[0]), len(buf)); err == nil {
				rv.manufacturer = syscall.UTF16ToString(buf)
			}

			if _, err := call(_HidD_GetProductString, f, unsafe.Pointer(&buf[0]), len(buf)); err == nil {
				rv.product = syscall.UTF16ToString(buf)
			}

			if _, err := call(_HidD_GetSerialNumberString, f, unsafe.Pointer(&buf[0]), len(buf)); err == nil {
				rv.serialNumber = syscall.UTF16ToString(buf)
			}

			var preparsed uintptr
			if b, err := call(_HidD_GetPreparsedData, f, unsafe.Pointer(&preparsed)); err != nil || b == 0 {
				return nil
			}
			defer call(_HidD_FreePreparsedData, preparsed)

			var caps _HIDP_CAPS
			if status, err := call(_HidP_GetCaps, preparsed, unsafe.Pointer(&caps)); err != nil || uint32(status) != _HIDP_STATUS_SUCCESS {
				return nil
			}

			rv.usagePage = caps.usagePage
			rv.usage = caps.usage
			rv.reportInputLength = caps.inputReportByteLength - 1
			rv.reportOutputLength = caps.outputReportByteLength - 1
			rv.reportFeatureLength = caps.featureReportByteLength - 1
			rv.reportWithID = true
			return rv
		}()

		if d != nil {
			rv = append(rv, d)
		}
	}

	return rv, nil
}

func (d *Device) open(lock bool) error {
	pathW, err := syscall.UTF16FromString(d.path)
	if err != nil {
		return nil
	}

	f, err := call(_CreateFileW, unsafe.Pointer(&pathW[0]), _GENERIC_READ|_GENERIC_WRITE, _FILE_SHARE_READ|_FILE_SHARE_WRITE, null, _OPEN_EXISTING, _FILE_FLAG_OVERLAPPED, null)
	if err != nil {
		return err
	}
	d.extra.file = f

	if lock {
		return d.lock()
	}
	return nil
}

func (d *Device) lock() error {
	hash := sha1.Sum([]byte(d.path))
	lockFile := path.Join(os.TempDir(), "usbhid-"+hex.EncodeToString(hash[:]))
	if maxPath := 260 - len(".lock") - 1; len(lockFile) > maxPath {
		lockFile = lockFile[:maxPath]
	}
	lockFile += ".lock"

	err := func() error {
		if err := os.WriteFile(lockFile, []byte{}, 0777); err != nil {
			return err
		}

		pathW, err := syscall.UTF16FromString(lockFile)
		if err != nil {
			return nil
		}

		f, err := call(_CreateFileW, unsafe.Pointer(&pathW[0]), _GENERIC_READ|_GENERIC_WRITE, _FILE_SHARE_READ|_FILE_SHARE_WRITE, null, _OPEN_EXISTING, _FILE_FLAG_OVERLAPPED, null)
		if err != nil {
			return err
		}

		if _, err = call(_LockFile, f, _LOCKFILE_EXCLUSIVE_LOCK|_LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0); err != nil {
			if _, err_ := call(_CloseHandle, f); err_ != nil {
				return err_
			}
			return err
		}
		d.extra.flock = f
		d.extra.flockPath = lockFile
		return nil
	}()

	if err != nil {
		if errno, match := err.(syscall.Errno); match && errno == _ERROR_LOCK_VIOLATION {
			return ErrDeviceLocked
		}
	}
	return err
}

func (d *Device) isOpen() bool {
	return d.extra.file != null
}

func (d *Device) close() error {
	if _, err := call(_CloseHandle, d.extra.file); err != nil {
		return err
	}
	d.extra.file = null

	if d.extra.flock != null {
		if _, err := call(_CloseHandle, d.extra.flock); err != nil {
			return err
		}
		d.extra.flock = null
		if d.extra.flockPath != "" {
			if err := os.Remove(d.extra.flockPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Device) getInputReport() (byte, []byte, error) {
	ovl := overlapped{}
	buf := make([]byte, d.reportInputLength+1)
	if _, err := call(_ReadFile, d.extra.file, unsafe.Pointer(&buf[0]), len(buf), 0, &ovl); err != nil {
		return 0, nil, err
	}
	return buf[0], buf[1:ovl.n], nil
}

func (d *Device) setOutputReport(reportID byte, data []byte) error {
	buf := data
	if len(buf) >= int(d.reportOutputLength) {
		buf = buf[:d.reportOutputLength]
	} else {
		buf = append(buf, make([]byte, int(d.reportOutputLength)-len(buf))...)
	}
	buf = append([]byte{reportID}, buf...)

	_, err := call(_WriteFile, d.extra.file, unsafe.Pointer(&buf[0]), len(buf), 0, &overlapped{})
	return err
}

func (d *Device) getFeatureReport(reportID byte) ([]byte, error) {
	buf := make([]byte, d.reportFeatureLength+1)
	buf[0] = reportID

	n, err := ioctl(d.extra.file, _IOCTL_HID_GET_FEATURE, nil, buf)
	if err != nil {
		return nil, err
	}
	return buf[1:n], nil
}

func (d *Device) setFeatureReport(reportID byte, data []byte) error {
	buf := append([]byte{reportID}, data...)

	_, err := ioctl(d.extra.file, _IOCTL_HID_SET_FEATURE, buf, nil)
	return err
}
