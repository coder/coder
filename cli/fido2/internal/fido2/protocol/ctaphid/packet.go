package ctaphid

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/coder/coder/v2/cli/fido2/internal/fido2/transport/hid"
)

// Message is a sequence of packets that form a complete CTAPHID message.
type Message []*packet

// NewMessage creates a new CTAPHID message from a channel ID, command, and data payload.
// It automatically splits the payload into multiple packets if necessary (segmentation).
func NewMessage(cid ChannelID, cmd Command, data []byte) (Message, error) {
	if len(data) > 7609 {
		return nil, ErrMessageTooLarge
	}

	msg := make(Message, 0)
	msg = append(msg, &packet{
		cid:     cid,
		command: cmd,
		length:  uint16(len(data)),
		// DATA starts from offset 7
		data: data[:min(len(data), 64-7)],
	})

	// if data is longer than 64 bytes minus offset, split it into chunks and
	// append them to the message as continuation packets
	if len(data) > (64 - 7) {
		var seq int
		for chunk := range slices.Chunk(data[64-7:], 64-5) {
			msg = append(msg, &packet{
				cid:          cid,
				sequence:     byte(seq),
				data:         chunk,
				continuation: true,
			})
			seq++
		}
	}

	return msg, nil
}

// WriteTo writes the message to the USB HID device.
// It serializes each packet and sends it as a report.
func (m *Message) WriteTo(dev *hid.Device) (int64, error) {
	var total int64
	for _, p := range *m {
		buf := bytes.NewBuffer(make([]byte, 0, PacketSize))

		// Packet
		n, err := p.WriteTo(buf)
		if err != nil {
			return 0, err
		}
		total += n

		// Pad with zeros to PacketSize
		if buf.Len() < PacketSize {
			padding := make([]byte, PacketSize-buf.Len())
			if _, err := buf.Write(padding); err != nil {
				return 0, err
			}
		}

		// Write to the device
		if err := dev.SetOutputReport(0x00, buf.Bytes()); err != nil {
			return 0, err
		}
	}

	return total, nil
}

// ReadFrom reads a complete CTAPHID message from the USB HID device.
// It reads packets from the device until the full message is reassembled.
func (m *Message) ReadFrom(dev *hid.Device) (int64, error) {
	var bytesRead int

	total := -1
	var expectedCID ChannelID
	var nextSeq byte
	gotInit := false
	for total != 0 {
		_, data, err := dev.GetInputReport()
		if err != nil {
			return 0, err
		}

		buf := bufio.NewReaderSize(bytes.NewReader(data), PacketSize)

		var p packet

		cid := make([]byte, 4)
		cidCnt, err := buf.Read(cid)
		if err != nil {
			return 0, err
		}
		bytesRead += cidCnt
		if cidCnt != 4 {
			return 0, errors.New("invalid cid length")
		}
		p.cid = ChannelID(cid)

		cmdOrSeq, err := buf.ReadByte()
		if err != nil {
			return 0, err
		}
		cmdOrSeqCnt := 1
		bytesRead += cmdOrSeqCnt

		if (cmdOrSeq & InitPacketBit) != 0 {
			p.command = Command(cmdOrSeq & ^InitPacketBit)
		} else {
			p.sequence = cmdOrSeq
			p.continuation = true
		}

		dataLenCnt := 0
		if !p.continuation {
			dataLen := make([]byte, 2)
			cnt, err := buf.Read(dataLen)
			if err != nil {
				return 0, err
			}
			bytesRead += cnt
			p.length = binary.BigEndian.Uint16(dataLen)
			if int(p.length) > 7609 {
				return 0, ErrMessageTooLarge
			}
			total = int(p.length)
			dataLenCnt = cnt

			expectedCID = p.cid
			nextSeq = 0
			gotInit = true
		} else {
			if !gotInit {
				return 0, ErrInvalidResponseMessage
			}
			if p.cid != expectedCID {
				return 0, ErrInvalidCID
			}
			if p.sequence != nextSeq {
				return 0, ErrInvalidResponseMessage
			}
			nextSeq++
		}

		dataCnt := total
		if total > 64-(cidCnt+cmdOrSeqCnt+dataLenCnt) {
			dataCnt = 64 - (cidCnt + cmdOrSeqCnt + dataLenCnt)
		}

		p.data = make([]byte, dataCnt)
		dataCnt, err = buf.Read(p.data)
		if err != nil {
			return 0, err
		}
		bytesRead += dataCnt

		total -= dataCnt
		*m = append(*m, &p)
	}

	return int64(bytesRead), nil
}

const (
	// PacketSize is the standard HID report size for FIDO.
	PacketSize = 64
	// InitPacketBit is the bit to identify an init packet.
	InitPacketBit byte = 0x80
)

// packet represents a single CTAPHID packet (init or continuation).
type packet struct {
	cid          ChannelID
	command      Command
	sequence     byte
	length       uint16
	data         []byte
	continuation bool
}

// WriteTo writes the packet structure to an io.Writer.
func (p *packet) WriteTo(w io.Writer) (int64, error) {
	// CID: offset 0; length 4
	cidCnt, err := w.Write(p.cid[:])
	if err != nil {
		return 0, err
	}

	// CMD or SEQ: offset 4; length 1
	cmdOrSeqCnt := 0
	if !p.continuation {
		cmdCnt, err := w.Write([]byte{byte(p.command) | InitPacketBit})
		if err != nil {
			return 0, err
		}
		cmdOrSeqCnt = cmdCnt
	} else {
		seqCnt, err := w.Write([]byte{p.sequence})
		if err != nil {
			return 0, err
		}
		cmdOrSeqCnt = seqCnt
	}

	// BCNTH and BCNTL: offset 5; length 2
	// Only present in an init packet.
	dataLenCnt := 0
	if !p.continuation {
		dataLen := make([]byte, 2)
		binary.BigEndian.PutUint16(dataLen, p.length)
		cnt, err := w.Write(dataLen)
		if err != nil {
			return 0, err
		}
		dataLenCnt = cnt
	}

	// DATA:
	//   Init packet offset 7; length 57
	//   Continuation packet offset 5; length 59
	dataCnt, err := w.Write(p.data)
	if err != nil {
		return 0, err
	}

	return int64(cidCnt + cmdOrSeqCnt + dataLenCnt + dataCnt), nil
}

// ChannelID represents CTAP channel ID.
type ChannelID [4]byte

var (
	// BroadcastCID is the broadcast channel ID (0xffffffff).
	BroadcastCID = ChannelID{0xff, 0xff, 0xff, 0xff}
)

// CBORResponse represents CTAPHID_CBOR (0x10) command response.
// See: https://fidoalliance.org/specs/fido-v2.2-ps-20250714/fido-client-to-authenticator-protocol-v2.2-ps-20250714.html#usb-hid-cbor
type CBORResponse struct {
	StatusCode StatusCode
	Data       []byte
}

// CapabilityFlag represents flags indicating device capabilities.
type CapabilityFlag byte

const (
	// CapabilityWINK means the device supports the WINK command.
	CapabilityWINK CapabilityFlag = 0x02
	// CapabilityCBOR means the device supports the CBOR command.
	CapabilityCBOR CapabilityFlag = 0x04
	// CapabilityNMSG means the device does NOT support the MSG command.
	CapabilityNMSG CapabilityFlag = 0x08
)

// Error represents a CTAPHID level error code.
type Error byte

func (e Error) String() string {
	return errorsStringMap[e]
}

func (e Error) Error() string {
	return fmt.Sprintf("CTAPHID error: %s (0x%02X)", e.String(), byte(e))
}

const (
	// ErrorInvalidCmd means the command is invalid.
	ErrorInvalidCmd Error = 0x01
	// ErrorInvalidParam means a parameter is invalid.
	ErrorInvalidParam Error = 0x02
	// ErrorInvalidLen means the length is invalid.
	ErrorInvalidLen Error = 0x03
	// ErrorInvalidSeq means the sequence number is invalid.
	ErrorInvalidSeq Error = 0x04
	// ErrorMsgTimeout means the message timed out.
	ErrorMsgTimeout Error = 0x05
	// ErrorChannelBusy means the channel is busy.
	ErrorChannelBusy Error = 0x06
	// ErrorLockRequired means a lock is required.
	ErrorLockRequired Error = 0x0A
	// ErrorInvalidChannel means the channel is invalid.
	ErrorInvalidChannel Error = 0x0B
	// ErrorOther means some other error occurred.
	ErrorOther Error = 0x7F
)

var errorsStringMap = map[Error]string{
	ErrorInvalidCmd:     "Invalid command",
	ErrorInvalidParam:   "Invalid parameter",
	ErrorInvalidLen:     "Invalid length",
	ErrorInvalidSeq:     "Invalid sequence",
	ErrorMsgTimeout:     "Message timeout",
	ErrorChannelBusy:    "Channel busy",
	ErrorLockRequired:   "Lock required",
	ErrorInvalidChannel: "Invalid channel",
	ErrorOther:          "Other error",
}

// InitResponse represents CTAPHID_INIT (0x06) command response.
// See: https://fidoalliance.org/specs/fido-v2.2-ps-20250714/fido-client-to-authenticator-protocol-v2.2-ps-20250714.html#usb-hid-init
type InitResponse struct {
	Nonce                            []byte
	CID                              ChannelID
	CTAPHIDProtocolVersionIdentifier byte
	MajorDeviceVersion               byte
	MinorDeviceVersion               byte
	BuildDeviceVersion               byte
	CapabilityFlags                  byte
}

// ImplementsWink returns true if the device supports the WINK command.
func (r *InitResponse) ImplementsWink() bool {
	return r.CapabilityFlags&byte(CapabilityWINK) != 0
}

// ImplementsCBOR returns true if the device supports the CBOR command.
func (r *InitResponse) ImplementsCBOR() bool {
	return r.CapabilityFlags&byte(CapabilityCBOR) != 0
}

// NotImplementsMSG returns true if the device does NOT support the MSG command.
func (r *InitResponse) NotImplementsMSG() bool {
	return r.CapabilityFlags&byte(CapabilityNMSG) != 0
}

// PingResponse represents CTAPHID_PING command response.
// See: https://fidoalliance.org/specs/fido-v2.2-ps-20250714/fido-client-to-authenticator-protocol-v2.2-ps-20250714.html#usb-hid-ping
type PingResponse struct {
	Bytes []byte
}

// ErrorResponse represents CTAPHID_ERROR (0x3F) command response.
type ErrorResponse struct {
	ErrorCode Error
}
