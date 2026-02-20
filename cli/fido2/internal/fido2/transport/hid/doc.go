// Package hid provides access to Human Interface Devices (HID).
//
// It is designed to be a simple, platform-independent interface for accessing
// HID devices. It currently supports Linux, macOS, and Windows.
//
// The package allows enumeration of connected devices, opening devices for
// exclusive access, and reading/writing input, output, and feature reports.
//
// Example usage:
//
//	ddevices, err := hid.Enumerate()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for _, d := range devices {
//	    fmt.Printf("Device: %s\n", d.Product())
//	    if d.VendorID() == 0x1234 && d.ProductID() == 0x5678 {
//	        if err := d.Open(true); err != nil {
//	            log.Fatal(err)
//	        }
//	        defer d.Close()
//
//	        // Read/Write reports...
//	    }
//	}
package hid
