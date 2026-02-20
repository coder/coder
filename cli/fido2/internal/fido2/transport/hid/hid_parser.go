package hid

import "math"

// hidValue parses a value from the descriptor buffer based on its size.
func hidValue(size byte, buf []byte) uint32 {
	switch size {
	case 0:
		return 0
	case 1:
		return uint32(buf[0])
	case 2:
		return uint32(buf[1])<<8 | uint32(buf[0])
	case 4:
		return uint32(buf[3])<<24 | uint32(buf[2])<<16 | uint32(buf[1])<<8 | uint32(buf[0])
	}
	return 0
}

// maxReportLength calculates the maximum size required for a report type (input, output, feature).
func maxReportLength(m map[uint32][]uint32) uint16 {
	var rv uint16
	for _, es := range m {
		var sum float64
		for _, e := range es {
			sum += float64(e)
		}
		if res := uint16(math.Ceil(sum / 8)); res > rv {
			rv = res
		}
	}
	return rv
}

// hidParseReportDescriptor parses a HID report descriptor to extract
// usage page, usage, and report lengths for input, output, and feature reports.
// It returns:
// - Usage Page
// - Usage
// - Input Report Length (in bytes)
// - Output Report Length (in bytes)
// - Feature Report Length (in bytes)
// - Whether the device uses Report IDs.
func hidParseReportDescriptor(descriptor []byte) (uint16, uint16, uint16, uint16, uint16, bool) {
	var (
		withID       bool
		rcollectionl byte
		rcount       uint32
		rsize        uint32
		rusagePage   uint32
		rusage       uint32
		rid          uint32

		input   = map[uint32][]uint32{}
		output  = map[uint32][]uint32{}
		feature = map[uint32][]uint32{}
	)

	for i := 0; i < len(descriptor); {
		var (
			tag  = (descriptor[i] & 0b11110000) >> 4
			typ  = (descriptor[i] & 0b1100) >> 2
			size = descriptor[i] & 0b11
		)

		i++

		if size == 3 {
			size = 4
		}

		switch typ {
		case 0: // main
			switch tag {
			case 8: // input
				input[rid] = append(input[rid], rcount*rsize)

			case 9: // output
				output[rid] = append(output[rid], rcount*rsize)

			case 10: // collection
				rcollectionl++

			case 11: // feature
				feature[rid] = append(feature[rid], rcount*rsize)

			case 12: // collection end
				rcollectionl--
			}

		case 1: // global
			switch tag {
			case 0: // usage page
				if rcollectionl == 0 {
					rusagePage = hidValue(size, descriptor[i:])
				}

			case 7: // report size
				rsize = hidValue(size, descriptor[i:])

			case 8: // report id
				rid = hidValue(size, descriptor[i:])
				withID = true

			case 9: // report count
				rcount = hidValue(size, descriptor[i:])
			}

		case 2: // local
			switch tag {
			case 0: // usage
				if rcollectionl == 0 {
					rusage = hidValue(size, descriptor[i:])
				}
			}
		}

		i += int(size)
	}

	//nolint:gosec // HID usage pages and usages fit in uint16 for our purposes
	return uint16(
			rusagePage,
		), uint16(
			rusage,
		), maxReportLength(
			input,
		), maxReportLength(
			output,
		), maxReportLength(
			feature,
		), withID
}
