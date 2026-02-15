package ctaphid

import (
	"crypto/subtle"
	"errors"
	"slices"

	"github.com/coder/coder/v2/cli/fido2/internal/fido2/transport/hid"
)

// Errors returned by the CTAPHID protocol.
var (
	ErrDeviceBusy = errors.New("device busy")
	ErrTimeout    = errors.New("device timeout")
	ErrInvalidCID = errors.New("invalid channel id")
)

// Command represents a CTAPHID command.
type Command byte

func (c Command) String() string {
	return commandStringMap[c]
}

// CTAPHID commands as defined in the specification.
const (
	// CmdMsg represents the CTAPHID_MSG command.
	CmdMsg Command = 0x03
	// CmdCBOR represents the CTAPHID_CBOR command.
	CmdCBOR Command = 0x10
	// CmdInit represents the CTAPHID_INIT command.
	CmdInit Command = 0x06
	// CmdPing represents the CTAPHID_PING command.
	CmdPing Command = 0x01
	// CmdCancel represents the CTAPHID_CANCEL command.
	CmdCancel Command = 0x11
	// CmdError represents the CTAPHID_ERROR command.
	CmdError Command = 0x3f
	// CmdKeepAlive represents the CTAPHID_KEEPALIVE command.
	CmdKeepAlive Command = 0x3b
	// CmdWink represents the CTAPHID_WINK command.
	CmdWink Command = 0x08
	// CmdLock represents the CTAPHID_LOCK command.
	CmdLock Command = 0x04
)

var commandStringMap = map[Command]string{
	CmdMsg:       "CTAPHID_MSG",
	CmdCBOR:      "CTAPHID_CBOR",
	CmdInit:      "CTAPHID_INIT",
	CmdPing:      "CTAPHID_PING",
	CmdCancel:    "CTAPHID_CANCEL",
	CmdError:     "CTAPHID_ERROR",
	CmdKeepAlive: "CTAPHID_KEEPALIVE",
	CmdWink:      "CTAPHID_WINK",
	CmdLock:      "CTAPHID_LOCK",
}

// StatusCode represents the status code returned by the authenticator in a CTAP2 response.
type StatusCode byte

func (s StatusCode) String() string {
	return statusCodeStringMap[s]
}

// CTAP status codes.
const (
	// StatusCTAP2OK means the operation was successful.
	StatusCTAP2OK StatusCode = 0x00
	// StatusCTAP1ErrInvalidCommand means the command is invalid.
	StatusCTAP1ErrInvalidCommand StatusCode = 0x01
	// StatusCTAP1ErrInvalidParameter means a parameter is invalid.
	StatusCTAP1ErrInvalidParameter StatusCode = 0x02
	// StatusCTAP1ErrInvalidLength means the length is invalid.
	StatusCTAP1ErrInvalidLength StatusCode = 0x03
	// StatusCTAP1ErrInvalidSeq means the sequence number is invalid.
	StatusCTAP1ErrInvalidSeq StatusCode = 0x04
	// StatusCTAP1ErrTimeout means the operation timed out.
	StatusCTAP1ErrTimeout StatusCode = 0x05
	// StatusCTAP1ErrChannelBusy means the channel is busy.
	StatusCTAP1ErrChannelBusy StatusCode = 0x06
	// StatusCTAP1ErrLockRequired means a lock is required.
	StatusCTAP1ErrLockRequired StatusCode = 0x0A
	// StatusCTAP1ErrInvalidChannel means the channel ID is invalid.
	StatusCTAP1ErrInvalidChannel StatusCode = 0x0B
	// StatusCTAP2ErrCBORUnexpectedType means an unexpected CBOR type was encountered.
	StatusCTAP2ErrCBORUnexpectedType StatusCode = 0x11
	// StatusCTAP2ErrInvalidCBOR means the CBOR is invalid.
	StatusCTAP2ErrInvalidCBOR StatusCode = 0x12
	// StatusCTAP2ErrMissingParameter means a required parameter is missing.
	StatusCTAP2ErrMissingParameter StatusCode = 0x14
	// StatusCTAP2ErrLimitExceeded means a limit was exceeded.
	StatusCTAP2ErrLimitExceeded StatusCode = 0x15
	// StatusCTAP2ErrFPDatabaseFull means the fingerprint database is full.
	StatusCTAP2ErrFPDatabaseFull StatusCode = 0x17
	// StatusCTAP2ErrLargeBlobStorageFull means the large blob storage is full.
	StatusCTAP2ErrLargeBlobStorageFull StatusCode = 0x18
	// StatusCTAP2ErrCredentialExcluded means the credential is excluded.
	StatusCTAP2ErrCredentialExcluded StatusCode = 0x19
	// StatusCTAP2ErrProcessing means the authenticator is still processing.
	StatusCTAP2ErrProcessing StatusCode = 0x21
	// StatusCTAP2ErrInvalidCredential means the credential is invalid.
	StatusCTAP2ErrInvalidCredential StatusCode = 0x22
	// StatusCTAP2ErrUserActionPending means a user action is pending.
	StatusCTAP2ErrUserActionPending StatusCode = 0x23
	// StatusCTAP2ErrOperationPending means an operation is pending.
	StatusCTAP2ErrOperationPending StatusCode = 0x24
	// StatusCTAP2ErrNoOperations means there are no operations.
	StatusCTAP2ErrNoOperations StatusCode = 0x25
	// StatusCTAP2ErrUnsupportedAlgorithm means the algorithm is unsupported.
	StatusCTAP2ErrUnsupportedAlgorithm StatusCode = 0x26
	// StatusCTAP2ErrOperationDenied means the operation was denied.
	StatusCTAP2ErrOperationDenied StatusCode = 0x27
	// StatusCTAP2ErrKeyStoreFull means the key store is full.
	StatusCTAP2ErrKeyStoreFull StatusCode = 0x28
	// StatusCTAP2ErrUnsupportedOption means the option is unsupported.
	StatusCTAP2ErrUnsupportedOption StatusCode = 0x2B
	// StatusCTAP2ErrInvalidOption means the option is invalid.
	StatusCTAP2ErrInvalidOption StatusCode = 0x2C
	// StatusCTAP2ErrKeepaliveCancel means the keepalive was canceled.
	StatusCTAP2ErrKeepaliveCancel StatusCode = 0x2D
	// StatusCTAP2ErrNoCredentials means no credentials were found.
	StatusCTAP2ErrNoCredentials StatusCode = 0x2E
	// StatusCTAP2ErrUserActionTimeout means the user action timed out.
	StatusCTAP2ErrUserActionTimeout StatusCode = 0x2F
	// StatusCTAP2ErrNotAllowed means the operation is not allowed.
	StatusCTAP2ErrNotAllowed StatusCode = 0x30
	// StatusCTAP2ErrPinInvalid means the PIN is invalid.
	StatusCTAP2ErrPinInvalid StatusCode = 0x31
	// StatusCTAP2ErrPinBlocked means the PIN is blocked.
	StatusCTAP2ErrPinBlocked StatusCode = 0x32
	// StatusCTAP2ErrPinAuthInvalid means the PIN auth is invalid.
	StatusCTAP2ErrPinAuthInvalid StatusCode = 0x33
	// StatusCTAP2ErrPinAuthBlocked means the PIN auth is blocked.
	StatusCTAP2ErrPinAuthBlocked StatusCode = 0x34
	// StatusCTAP2ErrPinNotSet means the PIN is not set.
	StatusCTAP2ErrPinNotSet StatusCode = 0x35
	// StatusCTAP2ErrPUATRequired means a PIN/UV auth token is required.
	StatusCTAP2ErrPUATRequired StatusCode = 0x36
	// StatusCTAP2ErrPinPolicyViolation means the PIN policy was violated.
	StatusCTAP2ErrPinPolicyViolation StatusCode = 0x37
	// StatusReservedForFutureUse is reserved for future use.
	StatusReservedForFutureUse StatusCode = 0x38
	// StatusCTAP2ErrRequestTooLarge means the request is too large.
	StatusCTAP2ErrRequestTooLarge StatusCode = 0x39
	// StatusCTAP2ErrActionTimeout means the action timed out.
	StatusCTAP2ErrActionTimeout StatusCode = 0x3A
	// StatusCTAP2ErrUpRequired means user presence is required.
	StatusCTAP2ErrUpRequired StatusCode = 0x3B
	// StatusCTAP2ErrUVBlocked means user verification is blocked.
	StatusCTAP2ErrUVBlocked StatusCode = 0x3C
	// StatusCTAP2ErrIntegrityFailure means integrity check failed.
	StatusCTAP2ErrIntegrityFailure StatusCode = 0x3D
	// StatusCTAP2ErrInvalidSubcommand means the subcommand is invalid.
	StatusCTAP2ErrInvalidSubcommand StatusCode = 0x3E
	// StatusCTAP2ErrUVInvalid means user verification is invalid.
	StatusCTAP2ErrUVInvalid StatusCode = 0x3F
	// StatusCTAP2ErrUnauthorizedPermission means the permission is unauthorized.
	StatusCTAP2ErrUnauthorizedPermission StatusCode = 0x40
	// StatusCTAP1ErrOther means some other error occurred.
	StatusCTAP1ErrOther StatusCode = 0x7F
	// StatusCTAP2ErrSpecLast is the last error code in the spec.
	StatusCTAP2ErrSpecLast StatusCode = 0xDF
	// StatusCTAP2ErrExtensionFirst is the first error code for extensions.
	StatusCTAP2ErrExtensionFirst StatusCode = 0xE0
	// StatusCTAP2ErrExtensionLast is the last error code for extensions.
	StatusCTAP2ErrExtensionLast StatusCode = 0xEF
	// StatusCTAP2ErrVendorFirst is the first error code for vendors.
	StatusCTAP2ErrVendorFirst StatusCode = 0xF0
	// StatusCTAP2ErrVendorLast is the last error code for vendors.
	StatusCTAP2ErrVendorLast StatusCode = 0xFF
)

var statusCodeStringMap = map[StatusCode]string{
	StatusCTAP2OK:                        "CTAP2_OK",
	StatusCTAP1ErrInvalidCommand:         "CTAP1_ERR_INVALID_COMMAND",
	StatusCTAP1ErrInvalidParameter:       "CTAP1_ERR_INVALID_PARAMETER",
	StatusCTAP1ErrInvalidLength:          "CTAP1_ERR_INVALID_LENGTH",
	StatusCTAP1ErrInvalidSeq:             "CTAP1_ERR_INVALID_SEQ",
	StatusCTAP1ErrTimeout:                "CTAP1_ERR_TIMEOUT",
	StatusCTAP1ErrChannelBusy:            "CTAP1_ERR_CHANNEL_BUSY",
	StatusCTAP1ErrLockRequired:           "CTAP1_ERR_LOCK_REQUIRED",
	StatusCTAP1ErrInvalidChannel:         "CTAP1_ERR_INVALID_CHANNEL",
	StatusCTAP2ErrCBORUnexpectedType:     "CTAP2_ERR_CBOR_UNEXPECTED_TYPE",
	StatusCTAP2ErrInvalidCBOR:            "CTAP2_ERR_INVALID_CBOR",
	StatusCTAP2ErrMissingParameter:       "CTAP2_ERR_MISSING_PARAMETER",
	StatusCTAP2ErrLimitExceeded:          "CTAP2_ERR_LIMIT_EXCEEDED",
	StatusCTAP2ErrFPDatabaseFull:         "CTAP2_ERR_FP_DATABASE",
	StatusCTAP2ErrLargeBlobStorageFull:   "CTAP2_ERR_LARGE_BLOB_STORAGE_FULL",
	StatusCTAP2ErrCredentialExcluded:     "CTAP2_ERR_CREDENTIAL_EXCLUDED",
	StatusCTAP2ErrProcessing:             "CTAP2_ERR_PROCESSING",
	StatusCTAP2ErrInvalidCredential:      "CTAP2_ERR_INVALID_CREDENTIAL",
	StatusCTAP2ErrUserActionPending:      "CTAP2_ERR_USER_ACTION_PENDING",
	StatusCTAP2ErrOperationPending:       "CTAP2_ERR_OPERATION_PENDING",
	StatusCTAP2ErrNoOperations:           "CTAP2_ERR_NO_OPERATIONS",
	StatusCTAP2ErrUnsupportedAlgorithm:   "CTAP2_ERR_UNSUPPORTED_ALGORITHM",
	StatusCTAP2ErrOperationDenied:        "CTAP2_ERR_OPERATION_DENIED",
	StatusCTAP2ErrKeyStoreFull:           "CTAP2_ERR_KEY_STORE_FULL",
	StatusCTAP2ErrUnsupportedOption:      "CTAP2_ERR_UNSUPPORTED_OPTION",
	StatusCTAP2ErrInvalidOption:          "CTAP2_ERR_INVALID_OPTION",
	StatusCTAP2ErrKeepaliveCancel:        "CTAP2_ERR_KEEPALIVE_CANCEL",
	StatusCTAP2ErrNoCredentials:          "CTAP2_ERR_NO_CREDENTIALS",
	StatusCTAP2ErrUserActionTimeout:      "CTAP2_ERR_USER_ACTION_TIMEOUT",
	StatusCTAP2ErrNotAllowed:             "CTAP2_ERR_NOT_ALLOWED",
	StatusCTAP2ErrPinInvalid:             "CTAP2_ERR_PIN_INVALID",
	StatusCTAP2ErrPinBlocked:             "CTAP2_ERR_PIN_BLOCKED",
	StatusCTAP2ErrPinAuthInvalid:         "CTAP2_ERR_PIN_AUTH_INVALID",
	StatusCTAP2ErrPinAuthBlocked:         "CTAP2_ERR_PIN_AUTH_BLOCKED",
	StatusCTAP2ErrPinNotSet:              "CTAP2_ERR_PIN_NOT_SET",
	StatusCTAP2ErrPUATRequired:           "CTAP2_ERR_PUAT_REQUIRED",
	StatusCTAP2ErrPinPolicyViolation:     "CTAP2_ERR_PIN_POLICY_VIOLATION",
	StatusReservedForFutureUse:           "RESERVED_FOR_FUTURE_USE",
	StatusCTAP2ErrRequestTooLarge:        "CTAP2_ERR_REQUEST_TOO_LARGE",
	StatusCTAP2ErrActionTimeout:          "CTAP2_ERR_ACTION_TIMEOUT",
	StatusCTAP2ErrUpRequired:             "CTAP2_ERR_UP_REQUIRED",
	StatusCTAP2ErrUVBlocked:              "CTAP2_ERR_UV_BLOCKED",
	StatusCTAP2ErrIntegrityFailure:       "CTAP2_ERR_INTEGRITY_FAILURE",
	StatusCTAP2ErrInvalidSubcommand:      "CTAP2_ERR_INVALID_SUBCOMMAND",
	StatusCTAP2ErrUVInvalid:              "CTAP2_ERR_UV_INVALID",
	StatusCTAP2ErrUnauthorizedPermission: "CTAP2_ERR_UNAUTHORIZED_PERMISSION",
	StatusCTAP1ErrOther:                  "CTAP1_ERR_OTHER",
	StatusCTAP2ErrSpecLast:               "CTAP2_ERR_SPEC_LAST",
	StatusCTAP2ErrExtensionFirst:         "CTAP2_ERR_EXTENSION_FIRST",
	StatusCTAP2ErrExtensionLast:          "CTAP2_ERR_EXTENSION_LAST",
	StatusCTAP2ErrVendorFirst:            "CTAP2_ERR_VENDOR_FIRST",
	StatusCTAP2ErrVendorLast:             "CTAP2_ERR_VENDOR_LAST",
}

// Client handles the CTAPHID protocol framing and communication with the device.
type Client struct {
	dev *hid.Device
}

// NewClient creates a new CTAPHID client and initializes the channel.
func NewClient(dev *hid.Device) *Client {
	return &Client{
		dev: dev,
	}
}

// Close closes the underlying HID connection.
func (c *Client) Close() error {
	return c.dev.Close()
}

// Init sends a CTAPHID_INIT command to the device to allocate a new channel.
func (c *Client) Init(cid ChannelID, nonce []byte) (*InitResponse, error) {
	msg, err := NewMessage(cid, CmdInit, nonce)
	if err != nil {
		return nil, err
	}

	if _, err := msg.WriteTo(c.dev); err != nil {
		return nil, err
	}

	for {
		respMsg := make(Message, 0)
		if _, err := respMsg.ReadFrom(c.dev); err != nil {
			return nil, err
		}

		if len(respMsg) < 1 {
			return nil, ErrInvalidResponseMessage
		}

		p := respMsg[0]

		switch p.command {
		case CmdInit:
			if len(p.data) < 17 {
				return nil, ErrInvalidResponseMessage
			}
			if subtle.ConstantTimeCompare(p.data[:8], nonce) != 1 {
				return nil, errors.New("invalid nonce")
			}

			r := &InitResponse{
				Nonce:                            p.data[:8],
				CID:                              ChannelID(p.data[8 : 8+4]),
				CTAPHIDProtocolVersionIdentifier: p.data[12],
				MajorDeviceVersion:               p.data[13],
				MinorDeviceVersion:               p.data[14],
				BuildDeviceVersion:               p.data[15],
				CapabilityFlags:                  p.data[16],
			}

			return r, nil
		case CmdError:
			return nil, Error(p.data[0])
		case CmdKeepAlive:
			continue
		default:
			return nil, ErrUnexpectedCommand
		}
	}
}

// Cancel sends a CTAPHID_CANCEL command to the device to cancel the current operation.
func (c *Client) Cancel(cid ChannelID) error {
	msg, err := NewMessage(cid, CmdCancel, nil)
	if err != nil {
		return err
	}

	if _, err := msg.WriteTo(c.dev); err != nil {
		return err
	}

	return nil
}

// Ping sends a CTAPHID_PING command to the device with the provided data.
func (c *Client) Ping(cid ChannelID, ping []byte) (*PingResponse, error) {
	msg, err := NewMessage(cid, CmdPing, ping)
	if err != nil {
		return nil, err
	}

	if _, err := msg.WriteTo(c.dev); err != nil {
		return nil, err
	}

read:
	for {
		respMsg := make(Message, 0)
		if _, err := respMsg.ReadFrom(c.dev); err != nil {
			return nil, err
		}

		if len(respMsg) < 1 {
			return nil, ErrInvalidResponseMessage
		}

		var pong []byte
		for i, p := range respMsg {
			if i == 0 {
				switch p.command {
				case CmdPing:
				case CmdError:
					return nil, Error(p.data[0])
				case CmdKeepAlive:
					continue read
				default:
					return nil, ErrUnexpectedCommand
				}
			}

			pong = slices.Concat(pong, p.data)
		}

		r := &PingResponse{
			Bytes: pong,
		}

		return r, nil
	}
}

// Wink sends a CTAPHID_WINK command to the device.
// This command performs a manufacturer-defined action (e.g., flashing an LED).
func (c *Client) Wink(cid ChannelID) error {
	msg, err := NewMessage(cid, CmdWink, nil)
	if err != nil {
		return err
	}

	if _, err := msg.WriteTo(c.dev); err != nil {
		return err
	}

	for {
		respMsg := make(Message, 0)
		if _, err := respMsg.ReadFrom(c.dev); err != nil {
			return err
		}

		if len(respMsg) < 1 {
			return ErrInvalidResponseMessage
		}

		p := respMsg[0]

		switch p.command {
		case CmdWink:
			return nil
		case CmdError:
			return Error(p.data[0])
		case CmdKeepAlive:
			continue
		default:
			return ErrUnexpectedCommand
		}
	}
}

// Lock sends a CTAPHID_LOCK command to the device to acquire an exclusive lock.
func (c *Client) Lock(cid ChannelID, seconds uint8) error {
	msg, err := NewMessage(cid, CmdLock, []byte{seconds})
	if err != nil {
		return err
	}

	if _, err := msg.WriteTo(c.dev); err != nil {
		return err
	}

	for {
		respMsg := make(Message, 0)
		if _, err := respMsg.ReadFrom(c.dev); err != nil {
			return err
		}

		if len(respMsg) < 1 {
			return ErrInvalidResponseMessage
		}

		p := respMsg[0]

		switch p.command {
		case CmdLock:
			return nil
		case CmdError:
			return Error(p.data[0])
		case CmdKeepAlive:
			continue
		default:
			return ErrUnexpectedCommand
		}
	}
}

// CBOR sends a CTAPHID_CBOR command to the device with the provided payload.
func (c *Client) CBOR(cid ChannelID, data []byte) (*CBORResponse, error) {
	msg, err := NewMessage(cid, CmdCBOR, data)
	if err != nil {
		return nil, err
	}

	if _, err := msg.WriteTo(c.dev); err != nil {
		return nil, err
	}

read:
	for {
		respMsg := make(Message, 0)
		if _, err := respMsg.ReadFrom(c.dev); err != nil {
			return nil, err
		}

		if len(respMsg) < 1 {
			return nil, ErrInvalidResponseMessage
		}

		var respData []byte
		for i, p := range respMsg {
			if i == 0 {
				switch p.command {
				case CmdCBOR:
					command := Command(data[0])
					code := StatusCode(p.data[0])
					if code != StatusCTAP2OK {
						return nil, newCTAPError(command, code)
					}
				case CmdError:
					return nil, Error(p.data[0])
				case CmdKeepAlive:
					continue read
				default:
					return nil, ErrUnexpectedCommand
				}
			}

			respData = slices.Concat(respData, p.data)
		}

		r := &CBORResponse{
			StatusCode: StatusCode(respData[0]),
			Data:       respData[1:],
		}

		return r, nil
	}
}
