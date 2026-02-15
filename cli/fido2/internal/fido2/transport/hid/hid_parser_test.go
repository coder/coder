package hid

import "testing"

var (
	b8 = []byte{
		0x05, 0x0C, // UsagePage(Consumer[0x000C])
		0x09, 0x01, // UsageId(Consumer Control[0x0001])
		0xA1, 0x01, // Collection(Application)
		0x85, 0x01, //     ReportId(1)
		0x09, 0x03, //     UsageId(Programmable Buttons[0x0003])
		0xA1, 0x02, //     Collection(Logical)
		0x05, 0x09, //         UsagePage(Button[0x0009])
		0x19, 0x01, //         UsageIdMin(Button 1[0x0001])
		0x29, 0x08, //         UsageIdMax(Button 8[0x0008])
		0x15, 0x00, //         LogicalMinimum(0)
		0x25, 0x01, //         LogicalMaximum(1)
		0x95, 0x08, //         ReportCount(8)
		0x75, 0x01, //         ReportSize(1)
		0x81, 0x02, //         Input(Data, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, BitField)
		0xC0,       //     EndCollection()
		0x05, 0x08, //     UsagePage(LED[0x0008])
		0x09, 0x3C, //     UsageId(Usage Multi Mode Indicator[0x003C])
		0xA1, 0x02, //     Collection(Logical)
		0x19, 0x3D, //         UsageIdMin(Indicator On[0x003D])
		0x29, 0x41, //         UsageIdMax(Indicator Off[0x0041])
		0x15, 0x01, //         LogicalMinimum(1)
		0x25, 0x05, //         LogicalMaximum(5)
		0x95, 0x01, //         ReportCount(1)
		0x75, 0x03, //         ReportSize(3)
		0x91, 0x00, //         Output(Data, Array, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0xC0,       //     EndCollection()
		0x75, 0x05, //     ReportSize(5)
		0x91, 0x03, //     Output(Constant, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0xC0, // EndCollection()
	}

	b8Mega = []byte{
		0x05, 0x0C, // UsagePage(Consumer[0x000C])
		0x09, 0x01, // UsageId(Consumer Control[0x0001])
		0xA1, 0x01, // Collection(Application)
		0x85, 0x01, //     ReportId(1)
		0x09, 0x03, //     UsageId(Programmable Buttons[0x0003])
		0xA1, 0x02, //     Collection(Logical)
		0x05, 0x09, //         UsagePage(Button[0x0009])
		0x19, 0x01, //         UsageIdMin(Button 1[0x0001])
		0x29, 0x08, //         UsageIdMax(Button 8[0x0008])
		0x15, 0x00, //         LogicalMinimum(0)
		0x25, 0x01, //         LogicalMaximum(1)
		0x95, 0x08, //         ReportCount(8)
		0x75, 0x01, //         ReportSize(1)
		0x81, 0x02, //         Input(Data, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, BitField)
		0xC0,       //     EndCollection()
		0x05, 0x08, //     UsagePage(LED[0x0008])
		0x09, 0x3C, //     UsageId(Usage Multi Mode Indicator[0x003C])
		0xA1, 0x02, //     Collection(Logical)
		0x19, 0x3D, //         UsageIdMin(Indicator On[0x003D])
		0x29, 0x41, //         UsageIdMax(Indicator Off[0x0041])
		0x15, 0x01, //         LogicalMinimum(1)
		0x25, 0x05, //         LogicalMaximum(5)
		0x95, 0x01, //         ReportCount(1)
		0x75, 0x03, //         ReportSize(3)
		0x91, 0x00, //         Output(Data, Array, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0xC0,       //     EndCollection()
		0x75, 0x05, //     ReportSize(5)
		0x91, 0x03, //     Output(Constant, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0x06, 0x00, 0xFF, //     UsagePage(b8[0xFF00])
		0x09, 0x01, //     UsageId(Capabilities[0x0001])
		0xA1, 0x02, //     Collection(Logical)
		0x09, 0x02, //         UsageId(With Display[0x0002])
		0x15, 0x00, //         LogicalMinimum(0)
		0x25, 0x01, //         LogicalMaximum(1)
		0x75, 0x01, //         ReportSize(1)
		0xB1, 0x03, //         Feature(Constant, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0xC0,       //     EndCollection()
		0x75, 0x07, //     ReportSize(7)
		0xB1, 0x03, //     Feature(Constant, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0x85, 0x02, //     ReportId(2)
		0x09, 0x11, //     UsageId(Display Capabilities[0x0011])
		0xA1, 0x02, //     Collection(Logical)
		0x09, 0x12, //         UsageId(Display Lines[0x0012])
		0x09, 0x13, //         UsageId(Display Characters per Line[0x0013])
		0x26, 0xFF, 0x00, //         LogicalMaximum(255)
		0x95, 0x02, //         ReportCount(2)
		0x75, 0x08, //         ReportSize(8)
		0xB1, 0x03, //         Feature(Constant, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0xC0,       //     EndCollection()
		0x09, 0x21, //     UsageId(Display Data[0x0021])
		0xA1, 0x02, //     Collection(Logical)
		0x09, 0x22, //         UsageId(Line[0x0022])
		0x25, 0x1F, //         LogicalMaximum(31)
		0x95, 0x01, //         ReportCount(1)
		0x75, 0x05, //         ReportSize(5)
		0x91, 0x02, //         Output(Data, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0x75, 0x03, //         ReportSize(3)
		0x91, 0x03, //         Output(Constant, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0x19, 0x32, //         UsageIdMin(Alignment Left[0x0032])
		0x29, 0x34, //         UsageIdMax(Alignment Center[0x0034])
		0x15, 0x01, //         LogicalMinimum(1)
		0x25, 0x03, //         LogicalMaximum(3)
		0x75, 0x02, //         ReportSize(2)
		0x91, 0x00, //         Output(Data, Array, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0x75, 0x06, //         ReportSize(6)
		0x91, 0x03, //         Output(Constant, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0x09, 0x23, //         UsageId(Line Data[0x0023])
		0x15, 0x00, //         LogicalMinimum(0)
		0x26, 0xFF, 0x00, //         LogicalMaximum(255)
		0x95, 0x15, //         ReportCount(21)
		0x75, 0x08, //         ReportSize(8)
		0x91, 0x02, //         Output(Data, Variable, Absolute, NoWrap, Linear, PreferredState, NoNullPosition, NonVolatile, BitField)
		0xC0, //     EndCollection()
		0xC0, // EndCollection()
	}

	args = []struct {
		descr      []byte
		rusagePage uint16
		rusage     uint16
		sinput     uint16
		soutput    uint16
		sfeature   uint16
		withID     bool
	}{
		{b8, 0x000C, 0x0001, 1, 1, 0, true},
		{b8Mega, 0x000C, 0x0001, 1, 23, 2, true},
	}
)

func TestParse(t *testing.T) {
	for i, tt := range args {
		rusagePage, rusage, sinput, soutput, sfeature, withID := hidParseReportDescriptor(tt.descr)

		if rusagePage != tt.rusagePage {
			t.Errorf("%d: bad usage page: got %q, want %q", i, rusagePage, tt.rusagePage)
		}
		if rusage != tt.rusage {
			t.Errorf("%d: bad usage: got %q, want %q", i, rusage, tt.rusage)
		}
		if sinput != tt.sinput {
			t.Errorf("%d: bad size input: got %d, want %d", i, sinput, tt.sinput)
		}
		if soutput != tt.soutput {
			t.Errorf("%d: bad size output: got %d, want %d", i, soutput, tt.soutput)
		}
		if sfeature != tt.sfeature {
			t.Errorf("%d: bad size feature: got %d, want %d", i, sfeature, tt.sfeature)
		}
		if withID != tt.withID {
			t.Errorf("%d: bad with id: got %v, want %v", i, withID, tt.withID)
		}
	}
}
