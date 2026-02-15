package hid

import (
	"errors"
	"fmt"
	"slices"
)

// FIDOUsagePage is the usage page for FIDO devices (0xF1D0).
const FIDOUsagePage uint16 = 0xF1D0

// Errors returned from usbhid package may be tested against these errors
// with errors.Is.
var (
	// ErrDeviceEnumerationFailed is returned when the device enumeration fails.
	ErrDeviceEnumerationFailed = errors.New("hid: usb hid device enumeration failed")
	// ErrDeviceFailedToClose is returned when the device fails to close.
	ErrDeviceFailedToClose = errors.New("hid: usb hid device failed to close")
	// ErrDeviceFailedToOpen is returned when the device fails to open.
	ErrDeviceFailedToOpen = errors.New("hid: usb hid device failed to open")
	// ErrDeviceIsClosed is returned when an operation is performed on a closed device.
	ErrDeviceIsClosed = errors.New("hid: usb hid device is closed")
	// ErrDeviceIsOpen is returned when an operation is performed on an already open device.
	ErrDeviceIsOpen = errors.New("hid: usb hid device is open")
	// ErrDeviceLocked is returned when the device is locked by another application.
	ErrDeviceLocked = errors.New("hid: usb hid device is locked by another application")
	// ErrGetFeatureReportFailed is returned when getting a feature report fails.
	ErrGetFeatureReportFailed = errors.New("hid: get usb hid feature report failed")
	// ErrGetInputReportFailed is returned when getting an input report fails.
	ErrGetInputReportFailed = errors.New("hid: get usb hid input report failed")
	// ErrMoreThanOneDeviceFound is returned when more than one device is found when expecting only one.
	ErrMoreThanOneDeviceFound = errors.New("hid: more than one usb hid device found")
	// ErrNoDeviceFound is returned when no device is found.
	ErrNoDeviceFound = errors.New("hid: no usb hid device found")
	// ErrReportBufferOverflow is returned when the report buffer overflows.
	ErrReportBufferOverflow = errors.New("hid: usb hid report buffer overflow")
	// ErrSetFeatureReportFailed is returned when setting a feature report fails.
	ErrSetFeatureReportFailed = errors.New("hid: set usb hid feature report failed")
	// ErrSetOutputReportFailed is returned when setting an output report fails.
	ErrSetOutputReportFailed = errors.New("hid: set usb hid output report failed")
)

// Device is an opaque structure that represents a USB HID device connected
// to the computer.
type Device struct {
	// path is the platform-specific device path or identifier.
	path         string
	vendorID     uint16
	productID    uint16
	version      uint16
	manufacturer string
	product      string
	serialNumber string

	// HID Report Descriptor information
	usagePage           uint16
	usage               uint16
	reportInputLength   uint16
	reportOutputLength  uint16
	reportFeatureLength uint16
	reportWithID        bool

	// Platform-specific extra data
	extra deviceExtra
}

// DeviceFilterFunc is a function alias that helps define a filter
// function to be used by the device enumeration functions.
type DeviceFilterFunc = func(*Device) bool

// Enumerate lists the USB HID devices connected to the computer.
func Enumerate() ([]*Device, error) {
	return EnumerateFilter(nil)
}

// EnumerateFilter lists the USB HID devices connected to the computer, filtered by a DeviceFilterFunc function.
func EnumerateFilter(f DeviceFilterFunc) ([]*Device, error) {
	devices, err := enumerate()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeviceEnumerationFailed, err)
	}

	if f == nil {
		return devices, nil
	}

	rv := []*Device{}
	for _, dev := range devices {
		if f(dev) {
			rv = append(rv, dev)
		}
	}
	return rv, nil
}

// Get returns a USB HID device found connected to the machine that matches the given path.
// It can optionally open the device and acquire an
// exclusive lock.
func Get(path string) (*Device, error) {
	devices, err := Enumerate()
	if err != nil {
		return nil, err
	}

	devices = slices.DeleteFunc(devices, func(device *Device) bool {
		return device.path != path
	})

	if l := len(devices); l == 0 {
		return nil, ErrNoDeviceFound
	}

	d := devices[0]

	return d, nil
}

// String returns a platform-independent string representation of the device.
func (d *Device) String() string {
	rv := fmt.Sprintf("vid=0x%04x; pid=0x%04x", d.vendorID, d.productID)
	if d.manufacturer != "" {
		rv += fmt.Sprintf("; mfr=%q", d.manufacturer)
	}
	if d.product != "" {
		rv += fmt.Sprintf("; prod=%q", d.product)
	}
	if d.serialNumber != "" {
		rv += fmt.Sprintf("; sn=%q", d.serialNumber)
	}
	return rv
}

// Open opens the USB HID device for usage.
func (d *Device) Open(lock bool) error {
	if d.isOpen() {
		return fmt.Errorf("%w [%s]", ErrDeviceIsOpen, d)
	}

	if err := d.open(lock); err != nil {
		if errors.Is(err, ErrDeviceLocked) {
			return fmt.Errorf("%w [%s]", ErrDeviceLocked, d)
		}
		return fmt.Errorf("%w [%s]: %w", ErrDeviceFailedToOpen, d, err)
	}
	return nil
}

// IsOpen checks if the USB HID device is open and available for usage.
func (d *Device) IsOpen() bool {
	return d.isOpen()
}

// Close closes the USB HID device.
func (d *Device) Close() error {
	if !d.isOpen() {
		return fmt.Errorf("%w [%s]", ErrDeviceIsClosed, d)
	}

	if err := d.close(); err != nil {
		return fmt.Errorf("%w [%s]: %w", ErrDeviceFailedToClose, d, err)
	}
	return nil
}

// GetInputReport reads an input report from the USB HID device.
// It will block until a report is available and returns the report id,
// a slice of bytes with the report content, and an error (or nil).
func (d *Device) GetInputReport() (byte, []byte, error) {
	if !d.isOpen() {
		return 0, nil, fmt.Errorf("%w [%s]", ErrDeviceIsClosed, d)
	}

	id, buf, err := d.getInputReport()
	if err != nil {
		return 0, nil, fmt.Errorf("%w [%s]: %w", ErrGetInputReportFailed, d, err)
	}
	return id, buf, nil
}

// SetOutputReport writes an output report to the USB HID device.
// It takes the report id and a slice of bytes with the data to be sent,
// and returns an error or nil. If the size of the slice is lower than
// the expected report size, it will be zero padded, and if it is bigger,
// an error is returned.
func (d *Device) SetOutputReport(reportID byte, data []byte) error {
	if !d.isOpen() {
		return fmt.Errorf("%w [%s]", ErrDeviceIsClosed, d)
	}

	if len(data) > int(d.reportOutputLength) {
		return fmt.Errorf("%w [%s]", ErrReportBufferOverflow, d)
	}

	if len(data) < int(d.reportOutputLength) {
		padded := make([]byte, d.reportOutputLength)
		copy(padded, data)
		data = padded
	}

	if err := d.setOutputReport(reportID, data); err != nil {
		return fmt.Errorf("%w [rid=%d; %s]: %w", ErrSetOutputReportFailed, reportID, d, err)
	}
	return nil
}

// GetFeatureReport reads a feature report from the USB HID device.
// It may block until a report is available, depending on the operating system.
// It takes the desired report id and returns a slice of bytes with the report
// content and an error (or nil).
func (d *Device) GetFeatureReport(reportID byte) ([]byte, error) {
	if !d.isOpen() {
		return nil, fmt.Errorf("%w [%s]", ErrDeviceIsClosed, d)
	}

	buf, err := d.getFeatureReport(reportID)
	if err != nil {
		return nil, fmt.Errorf("%w [rid=%d; %s]: %w", ErrGetFeatureReportFailed, reportID, d, err)
	}
	return buf, nil
}

// SetFeatureReport writes an output report to the USB HID device.
// It takes the report id and a slice of bytes with the data to be sent,
// and returns an error or nil. If the size of the slice is lower than
// the expected report size, it will be zero padded, and if it is bigger,
// an error is returned.
func (d *Device) SetFeatureReport(reportID byte, data []byte) error {
	if !d.isOpen() {
		return fmt.Errorf("%w [%s]", ErrDeviceIsClosed, d)
	}

	if len(data) > int(d.reportFeatureLength) {
		return fmt.Errorf("%w [%s]", ErrReportBufferOverflow, d)
	}

	if err := d.setFeatureReport(reportID, data); err != nil {
		return fmt.Errorf("%w [rid=%d; %s]: %w", ErrSetFeatureReportFailed, reportID, d, err)
	}
	return nil
}

// GetInputReportLength returns the data size of an input report in bytes.
func (d *Device) GetInputReportLength() uint16 {
	return d.reportInputLength
}

// GetOutputReportLength returns the data size of an output report in bytes.
func (d *Device) GetOutputReportLength() uint16 {
	return d.reportOutputLength
}

// GetFeatureReportLength returns the data size of a feature report in bytes.
func (d *Device) GetFeatureReportLength() uint16 {
	return d.reportFeatureLength
}

// Path returns a string representation of the USB HID device path.
func (d *Device) Path() string {
	return d.path
}

// VendorID returns the vendor identifier of the USB HID device.
func (d *Device) VendorID() uint16 {
	return d.vendorID
}

// ProductID returns the product identifier of the USB HID device.
func (d *Device) ProductID() uint16 {
	return d.productID
}

// Version returns a BCD representation of the product version of
// the USB HID device.
func (d *Device) Version() uint16 {
	return d.version
}

// Manufacturer returns a string representation of the manufacturer of
// the USB HID device.
func (d *Device) Manufacturer() string {
	return d.manufacturer
}

// Product returns a string representation of the product name of
// the USB HID device.
func (d *Device) Product() string {
	return d.product
}

// SerialNumber returns a string representation of the serial number of
// the USB HID device.
func (d *Device) SerialNumber() string {
	return d.serialNumber
}

// UsagePage returns the usage page of the USB HID device.
func (d *Device) UsagePage() uint16 {
	return d.usagePage
}

// Usage returns the usage identifier of the USB HID device.
func (d *Device) Usage() uint16 {
	return d.usage
}

// IsReportWithID indicates whether the USB HID device uses report IDs.
func (d *Device) IsReportWithID() bool {
	return d.reportWithID
}
