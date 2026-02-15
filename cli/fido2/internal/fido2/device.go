package fido2

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"iter"
	"slices"
	"sync"

	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/ctap2"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/ctaphid"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/webauthn"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/transport/hid"
	"github.com/fxamacker/cbor/v2"
	"github.com/ldclabs/cose/key"
)

// Device represents a FIDO2 device.
type Device struct {
	ctapClient ctap2.Client
	info       *ctap2.AuthenticatorGetInfoResponse
	mu         sync.Mutex
	closed     bool
}

// DeviceDescriptor provides information about a FIDO2 device.
// It is returned by the Enumerate function.
type DeviceDescriptor struct {
	// Path is the platform-specific device path.
	Path string
	// VendorID is the USB vendor identifier.
	VendorID uint16
	// ProductID is the USB product identifier.
	ProductID uint16
	// SerialNumber is the device serial number.
	SerialNumber string
	// Manufacturer is the device manufacturer name.
	Manufacturer string
	// Product is the device product name.
	Product string
}

// Enumerate returns a list of connected FIDO2 devices.
func Enumerate() ([]DeviceDescriptor, error) {
	hidDevs, err := hid.EnumerateFilter(func(d *hid.Device) bool {
		return d.UsagePage() == hid.FIDOUsagePage
	})

	if err != nil {
		return nil, fmt.Errorf("failed to enumerate devices: %w", err)
	}

	devDescs := make([]DeviceDescriptor, 0, len(hidDevs))
	for _, d := range hidDevs {
		devDescs = append(devDescs, DeviceDescriptor{
			Path:         d.Path(),
			VendorID:     d.VendorID(),
			ProductID:    d.ProductID(),
			SerialNumber: d.SerialNumber(),
			Manufacturer: d.Manufacturer(),
			Product:      d.Product(),
		})
	}

	return devDescs, nil
}

// Open opens a FIDO2 device using its descriptor.
func Open(descriptor DeviceDescriptor) (*Device, error) {
	return OpenPath(descriptor.Path)
}

// OpenPath opens a FIDO2 device by its platform-specific path.
func OpenPath(path string) (*Device, error) {
	hidDev, err := hid.Get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	if hidDev.UsagePage() != hid.FIDOUsagePage {
		return nil, fmt.Errorf("device at %s is not a FIDO device", path)
	}

	if err := hidDev.Open(false); err != nil {
		return nil, fmt.Errorf("failed to open device: %w", err)
	}

	ctaphidClient := ctaphid.NewClient(hidDev)

	encMode, err := cbor.CTAP2EncOptions().EncMode()
	if err != nil {
		hidDev.Close()
		return nil, fmt.Errorf("failed to create CBOR encoding mode: %w", err)
	}

	ctapClient, err := ctap2.NewClient(ctaphidClient, encMode)
	if err != nil {
		hidDev.Close()
		return nil, fmt.Errorf("failed to create CTAP2 client: %w", err)
	}

	info, err := ctapClient.GetInfo()
	if err != nil {
		hidDev.Close()
		return nil, fmt.Errorf("failed to get authenticator info: %w", err)
	}

	return &Device{
		ctapClient: ctapClient,
		mu:         sync.Mutex{},
		info:       info,
	}, nil
}

func (d *Device) pinUvAuthProtocol() (ctap2.PinUvAuthProtocolType, error) {
	if len(d.info.PinUvAuthProtocols) == 0 {
		return 0, newErrorMessage(ErrNotSupported, "device doesn't advertise pinUvAuthProtocols")
	}
	p := d.info.PinUvAuthProtocols[0]
	switch p {
	case ctap2.PinUvAuthProtocolTypeOne, ctap2.PinUvAuthProtocolTypeTwo:
		return p, nil
	default:
		return 0, ctap2.ErrInvalidPinAuthProtocol
	}
}

// Close closes the connection to the FIDO2 device.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.ctapClient.Close()
}

// MakeCredential initiates the process of creating a new credential.
func (d *Device) MakeCredential(
	pinUvAuthToken []byte,
	clientData []byte,
	rp webauthn.PublicKeyCredentialRpEntity,
	user webauthn.PublicKeyCredentialUserEntity,
	pubKeyCredParams []webauthn.PublicKeyCredentialParameters,
	excludeList []webauthn.PublicKeyCredentialDescriptor,
	extInputs *webauthn.CreateAuthenticationExtensionsClientInputs,
	options map[ctap2.Option]bool,
	enterpriseAttestation uint,
	attestationFormatsPreference []webauthn.AttestationStatementFormatIdentifier,
) (*ctap2.AuthenticatorMakeCredentialResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if extInputs == nil {
		extInputs = &webauthn.CreateAuthenticationExtensionsClientInputs{}
	}

	notRequired, ok := d.info.Options[ctap2.OptionMakeCredentialUvNotRequired]
	if (!ok || !notRequired) && pinUvAuthToken == nil {
		return nil, ErrPinUvAuthTokenRequired
	}

	var (
		pinProtocol    ctap2.PinUvAuthProtocolType
		pinProtocolSet bool
	)
	getPinProtocol := func() (ctap2.PinUvAuthProtocolType, error) {
		if pinProtocolSet {
			return pinProtocol, nil
		}
		p, err := d.pinUvAuthProtocol()
		if err != nil {
			return 0, err
		}
		pinProtocol = p
		pinProtocolSet = true
		return pinProtocol, nil
	}

	if pinUvAuthToken != nil {
		if _, err := getPinProtocol(); err != nil {
			return nil, err
		}
	}

	var (
		protocol     *ctap2.PinUvAuthProtocol
		sharedSecret []byte
	)

	extensions := new(ctap2.CreateExtensionInputs)

	if extInputs.LargeBlobInputs != nil {
		return nil, newErrorMessage(ErrSyntaxError, "largeBlob extension is not supported yet")
	}

	if extInputs.CreateHMACSecretMCInputs != nil && extInputs.PRFInputs != nil {
		return nil, newErrorMessage(ErrSyntaxError, "you cannot use hmac-secret and prf extensions at the same time")
	}

	// hmac-secret
	if extInputs.CreateHMACSecretInputs != nil {
		if !slices.Contains(d.info.Extensions, webauthn.ExtensionIdentifierHMACSecret) {
			return nil, newErrorMessage(ErrNotSupported, "device doesn't support hmac-secret extension")
		}

		extensions.CreateHMACSecretInput = &ctap2.CreateHMACSecretInput{
			HMACSecret: extInputs.HMACCreateSecret,
		}
	}

	// hmac-secret-mc
	if extInputs.CreateHMACSecretMCInputs != nil {
		pinProtocol, err := getPinProtocol()
		if err != nil {
			return nil, err
		}

		if !slices.Contains(d.info.Extensions, webauthn.ExtensionIdentifierHMACSecretMC) {
			return nil, newErrorMessage(ErrNotSupported, "device doesn't support hmac-secret-mc extension")
		}

		salt := slices.Concat(
			extInputs.HMACGetSecret.Salt1,
			extInputs.HMACGetSecret.Salt2,
		)

		protocol, err = ctap2.NewPinUvAuthProtocol(pinProtocol)
		if err != nil {
			return nil, err
		}

		keyAgreement, err := d.ctapClient.GetKeyAgreement(pinProtocol)
		if err != nil {
			return nil, err
		}

		var platformCoseKey key.Key
		platformCoseKey, sharedSecret, err = protocol.Encapsulate(keyAgreement)
		if err != nil {
			return nil, err
		}

		saltEnc, err := protocol.Encrypt(sharedSecret, salt)
		if err != nil {
			return nil, err
		}

		saltAuth, err := ctap2.AuthenticateWithError(pinProtocol, sharedSecret, saltEnc)
		if err != nil {
			return nil, err
		}

		extensions.CreateHMACSecretInput = &ctap2.CreateHMACSecretInput{
			HMACSecret: true,
		}
		extensions.CreateHMACSecretMCInput = &ctap2.CreateHMACSecretMCInput{
			HMACSecret: ctap2.HMACSecret{
				KeyAgreement:      platformCoseKey,
				SaltEnc:           saltEnc,
				SaltAuth:          saltAuth,
				PinUvAuthProtocol: pinProtocol,
			},
		}
	}

	// prf
	if extInputs.PRFInputs != nil {
		pinProtocol, err := getPinProtocol()
		if err != nil {
			return nil, err
		}

		if !slices.Contains(d.info.Extensions, webauthn.ExtensionIdentifierHMACSecretMC) {
			return nil, newErrorMessage(ErrNotSupported, "device doesn't support prf extension during registration")
		}

		if extInputs.PRF.EvalByCredential != nil {
			return nil, newErrorMessage(ErrNotSupported, "evalByCredential is not supported during registration")
		}

		if extInputs.PRF.Eval == nil {
			return nil, newErrorMessage(ErrSyntaxError, "eval is empty")
		}

		hasher := sha256.New()
		hasher.Write([]byte("WebAuthn PRF"))
		hasher.Write([]byte{0x00})
		hasher.Write(extInputs.PRF.Eval.First)
		salt := hasher.Sum(nil)

		if extInputs.PRF.Eval.Second != nil {
			hasher.Reset()
			hasher.Write([]byte("WebAuthn PRF"))
			hasher.Write([]byte{0x00})
			hasher.Write(extInputs.PRF.Eval.Second)
			salt = slices.Concat(salt, hasher.Sum(nil))
		}

		protocol, err = ctap2.NewPinUvAuthProtocol(pinProtocol)
		if err != nil {
			return nil, err
		}

		keyAgreement, err := d.ctapClient.GetKeyAgreement(pinProtocol)
		if err != nil {
			return nil, err
		}

		var platformCoseKey key.Key
		platformCoseKey, sharedSecret, err = protocol.Encapsulate(keyAgreement)
		if err != nil {
			return nil, err
		}

		saltEnc, err := protocol.Encrypt(sharedSecret, salt)
		if err != nil {
			return nil, err
		}

		saltAuth, err := ctap2.AuthenticateWithError(pinProtocol, sharedSecret, saltEnc)
		if err != nil {
			return nil, err
		}

		extensions.CreateHMACSecretInput = &ctap2.CreateHMACSecretInput{
			HMACSecret: true,
		}
		extensions.CreateHMACSecretMCInput = &ctap2.CreateHMACSecretMCInput{
			HMACSecret: ctap2.HMACSecret{
				KeyAgreement:      platformCoseKey,
				SaltEnc:           saltEnc,
				SaltAuth:          saltAuth,
				PinUvAuthProtocol: pinProtocol,
			},
		}
	}

	// credProtection
	if extInputs.CreateCredentialProtectionInputs != nil {
		var credProtect int

		switch extInputs.CredentialProtectionPolicy {
		case webauthn.CredentialProtectionPolicyUserVerificationOptional:
			credProtect = 0x01
		case webauthn.CredentialProtectionPolicyUserVerificationOptionalWithCredentialIDList:
			credProtect = 0x02
		case webauthn.CredentialProtectionPolicyUserVerificationRequired:
			credProtect = 0x03
		default:
			return nil, newErrorMessage(ErrNotSupported, "invalid credential protection policy")
		}

		if extInputs.EnforceCredentialProtectionPolicy &&
			extInputs.CredentialProtectionPolicy != webauthn.CredentialProtectionPolicyUserVerificationOptional &&
			!slices.Contains(d.info.Extensions, webauthn.ExtensionIdentifierCredentialProtection) {
			return nil, newErrorMessage(ErrNotSupported, "device doesn't support credProtect extension")
		}

		extensions.CreateCredProtectInput = &ctap2.CreateCredProtectInput{
			CredProtect: credProtect,
		}
	}

	// credBlob
	if extInputs.CreateCredentialBlobInputs != nil {
		if !slices.Contains(d.info.Extensions, webauthn.ExtensionIdentifierCredentialBlob) {
			return nil, newErrorMessage(ErrNotSupported, "device doesn't support credBlob extension")
		}

		if uint(len(extInputs.CredBlob)) > d.info.MaxCredBlobLength {
			return nil, newErrorMessage(
				ErrNotSupported,
				fmt.Sprintf("credBlob length must be less than %d bytes", d.info.MaxCredBlobLength),
			)
		}

		extensions.CreateCredBlobInput = &ctap2.CreateCredBlobInput{
			CredBlob: extInputs.CredBlob,
		}
	}

	// minPinLength
	if extInputs.CreateMinPinLengthInputs != nil {
		if !slices.Contains(d.info.Extensions, webauthn.ExtensionIdentifierMinPinLength) {
			return nil, newErrorMessage(ErrNotSupported, "device doesn't support minPinLength extension")
		}

		extensions.CreateMinPinLengthInput = &ctap2.CreateMinPinLengthInput{
			MinPinLength: extInputs.MinPinLength,
		}
	}
	if extInputs.CreatePinComplexityPolicyInputs != nil {
		if !slices.Contains(d.info.Extensions, webauthn.ExtensionIdentifierPinComplexityPolicy) {
			return nil, newErrorMessage(ErrNotSupported, "device doesn't support pinComplexityPolicy extension")
		}

		extensions.CreatePinComplexityPolicyInput = &ctap2.CreatePinComplexityPolicyInput{
			PinComplexityPolicy: extInputs.PinComplexityPolicy,
		}
	}

	pinProtocolArg := pinProtocol
	if !pinProtocolSet {
		pinProtocolArg = 0
	}

	resp, err := d.ctapClient.MakeCredential(
		pinProtocolArg,
		pinUvAuthToken,
		clientData,
		rp,
		user,
		pubKeyCredParams,
		excludeList,
		extensions,
		options,
		enterpriseAttestation,
		attestationFormatsPreference,
	)
	if err != nil {
		return nil, err
	}

	extOutputs := new(webauthn.CreateAuthenticationExtensionsClientOutputs)
	resp.ExtensionOutputs = extOutputs

	if extInputs.CreateCredentialProtectionInputs != nil && extInputs.CredentialProperties {
		extOutputs.CreateCredentialPropertiesOutputs = &webauthn.CreateCredentialPropertiesOutputs{
			CredentialProperties: webauthn.CredentialPropertiesOutput{
				ResidentKey: options[ctap2.OptionResidentKeys],
			},
		}
	}

	if !resp.AuthData.Flags.ExtensionDataIncluded() {
		return resp, nil
	}

	// credBlob
	if resp.AuthData.Extensions.CreateCredBlobOutput != nil {
		extOutputs.CreateCredentialBlobOutputs = &webauthn.CreateCredentialBlobOutputs{
			CredBlob: resp.AuthData.Extensions.CredBlob,
		}
	}

	// hmac-secret
	if resp.AuthData.Extensions.CreateHMACSecretOutput != nil {
		extOutputs.CreateHMACSecretOutputs = &webauthn.CreateHMACSecretOutputs{
			HMACCreateSecret: resp.AuthData.Extensions.CreateHMACSecretOutput.HMACSecret,
		}
	}

	// hmac-secret-mc (it needs tests, thought I cannot find any devices that support it yet)
	if resp.AuthData.Extensions.CreateHMACSecretMCOutput != nil {
		salt, err := protocol.Decrypt(sharedSecret, resp.AuthData.Extensions.CreateHMACSecretMCOutput.HMACSecret)
		if err != nil {
			return nil, err
		}

		switch len(salt) {
		case 32:
			extOutputs.PRFOutputs = &webauthn.PRFOutputs{
				PRF: webauthn.AuthenticationExtensionsPRFOutputs{
					Enabled: true,
					Results: webauthn.AuthenticationExtensionsPRFValues{
						First: salt[:32],
					},
				},
			}
		case 64:
			extOutputs.PRFOutputs = &webauthn.PRFOutputs{
				PRF: webauthn.AuthenticationExtensionsPRFOutputs{
					Enabled: true,
					Results: webauthn.AuthenticationExtensionsPRFValues{
						First:  salt[:32],
						Second: salt[32:],
					},
				},
			}
		default:
			return nil, newErrorMessage(ErrInvalidSaltSize, "salt must be 32 or 64 bytes")
		}
	}

	return resp, nil
}

// GetAssertion provides a generator function to iterate over assertions.
func (d *Device) GetAssertion(
	pinUvAuthToken []byte,
	rpID string,
	clientData []byte,
	allowList []webauthn.PublicKeyCredentialDescriptor,
	extInputs *webauthn.GetAuthenticationExtensionsClientInputs,
	options map[ctap2.Option]bool,
) iter.Seq2[*ctap2.AuthenticatorGetAssertionResponse, error] {
	return func(yield func(*ctap2.AuthenticatorGetAssertionResponse, error) bool) {
		d.mu.Lock()
		defer d.mu.Unlock()

		if extInputs == nil {
			extInputs = &webauthn.GetAuthenticationExtensionsClientInputs{}
		}

		var (
			pinProtocol    ctap2.PinUvAuthProtocolType
			pinProtocolSet bool
		)
		getPinProtocol := func() (ctap2.PinUvAuthProtocolType, error) {
			if pinProtocolSet {
				return pinProtocol, nil
			}
			p, err := d.pinUvAuthProtocol()
			if err != nil {
				return 0, err
			}
			pinProtocol = p
			pinProtocolSet = true
			return pinProtocol, nil
		}

		if pinUvAuthToken != nil {
			if _, err := getPinProtocol(); err != nil {
				yield(nil, err)
				return
			}
		}

		var (
			protocol     *ctap2.PinUvAuthProtocol
			sharedSecret []byte
		)

		extensions := new(ctap2.GetExtensionInputs)

		if extInputs.LargeBlobInputs != nil {
			yield(nil, newErrorMessage(ErrSyntaxError, "largeBlob extension is not supported yet"))
			return
		}

		if extInputs.PRFInputs != nil && extInputs.GetHMACSecretInputs != nil {
			yield(
				nil,
				newErrorMessage(ErrSyntaxError, "you cannot use hmac-secret and prf extensions at the same time"),
			)
			return
		}

		// hmac-secret
		if extInputs.GetHMACSecretInputs != nil {
			pinProtocol, err := getPinProtocol()
			if err != nil {
				yield(nil, err)
				return
			}

			salt := slices.Concat(
				extInputs.HMACGetSecret.Salt1,
				extInputs.HMACGetSecret.Salt2,
			)

			protocol, err = ctap2.NewPinUvAuthProtocol(pinProtocol)
			if err != nil {
				yield(nil, err)
				return
			}

			keyAgreement, err := d.ctapClient.GetKeyAgreement(pinProtocol)
			if err != nil {
				yield(nil, err)
				return
			}

			var platformCoseKey key.Key
			platformCoseKey, sharedSecret, err = protocol.Encapsulate(keyAgreement)
			if err != nil {
				yield(nil, err)
				return
			}

			saltEnc, err := protocol.Encrypt(sharedSecret, salt)
			if err != nil {
				yield(nil, err)
				return
			}

			saltAuth, err := ctap2.AuthenticateWithError(pinProtocol, sharedSecret, saltEnc)
			if err != nil {
				yield(nil, err)
				return
			}

			extensions.GetHMACSecretInput = &ctap2.GetHMACSecretInput{
				HMACSecret: ctap2.HMACSecret{
					KeyAgreement:      platformCoseKey,
					SaltEnc:           saltEnc,
					SaltAuth:          saltAuth,
					PinUvAuthProtocol: pinProtocol,
				},
			}
		}

		// prf
		if extInputs.PRFInputs != nil {
			pinProtocol, err := getPinProtocol()
			if err != nil {
				yield(nil, err)
				return
			}

			if extInputs.PRF.EvalByCredential != nil && (allowList == nil || len(allowList) == 0) {
				yield(
					nil,
					newErrorMessage(ErrNotSupported, "evalByCredential works only in conjunction with allowList"),
				)
				return
			}

			decodeCredentialID := func(idStr string) ([]byte, error) {
				if id, err := base64.RawURLEncoding.DecodeString(idStr); err == nil {
					return id, nil
				}
				return base64.URLEncoding.DecodeString(idStr)
			}

			var ev *webauthn.AuthenticationExtensionsPRFValues
			var ids [][]byte
			for idStr := range extInputs.PRF.EvalByCredential {
				id, err := decodeCredentialID(idStr)
				if err != nil {
					yield(nil, newErrorMessage(ErrSyntaxError, "invalid credential id"))
					return
				}

				ids = append(ids, id)
			}

			for _, id := range ids {
				index := slices.IndexFunc(allowList, func(descriptor webauthn.PublicKeyCredentialDescriptor) bool {
					if slices.Equal(descriptor.ID, id) {
						return true
					}

					return false
				})
				if index != -1 {
					rawKey := base64.RawURLEncoding.EncodeToString(allowList[index].ID)
					v, ok := extInputs.PRF.EvalByCredential[rawKey]
					if !ok {
						paddedKey := base64.URLEncoding.EncodeToString(allowList[index].ID)
						v, ok = extInputs.PRF.EvalByCredential[paddedKey]
					}
					if ok {
						ev = &v
					}
				}
			}

			if ev == nil && extInputs.PRF.Eval != nil {
				ev = extInputs.PRF.Eval
			}

			if ev == nil {
				yield(nil, newErrorMessage(ErrSyntaxError, "eval is empty"))
				return
			}

			hasher := sha256.New()
			hasher.Write([]byte("WebAuthn PRF"))
			hasher.Write([]byte{0x00})
			hasher.Write(ev.First)
			salt := hasher.Sum(nil)

			if ev.Second != nil {
				hasher.Reset()
				hasher.Write([]byte("WebAuthn PRF"))
				hasher.Write([]byte{0x00})
				hasher.Write(ev.Second)
				salt = slices.Concat(salt, hasher.Sum(nil))
			}

			protocol, err = ctap2.NewPinUvAuthProtocol(pinProtocol)
			if err != nil {
				yield(nil, err)
				return
			}

			keyAgreement, err := d.ctapClient.GetKeyAgreement(pinProtocol)
			if err != nil {
				yield(nil, err)
				return
			}

			var platformCoseKey key.Key
			platformCoseKey, sharedSecret, err = protocol.Encapsulate(keyAgreement)
			if err != nil {
				yield(nil, err)
				return
			}

			saltEnc, err := protocol.Encrypt(sharedSecret, salt)
			if err != nil {
				yield(nil, err)
				return
			}

			saltAuth, err := ctap2.AuthenticateWithError(pinProtocol, sharedSecret, saltEnc)
			if err != nil {
				yield(nil, err)
				return
			}

			extensions.GetHMACSecretInput = &ctap2.GetHMACSecretInput{
				HMACSecret: ctap2.HMACSecret{
					KeyAgreement:      platformCoseKey,
					SaltEnc:           saltEnc,
					SaltAuth:          saltAuth,
					PinUvAuthProtocol: pinProtocol,
				},
			}
		}

		// credBlob
		if extInputs.GetCredentialBlobInputs != nil {
			extensions.GetCredBlobInput = &ctap2.GetCredBlobInput{
				CredBlob: extInputs.GetCredBlob,
			}
		}

		pinProtocolArg := pinProtocol
		if !pinProtocolSet {
			pinProtocolArg = 0
		}

		for assertion, err := range d.ctapClient.GetAssertion(
			pinProtocolArg,
			pinUvAuthToken,
			rpID,
			clientData,
			allowList,
			extensions,
			options,
		) {
			if err != nil {
				yield(nil, err)
				return
			}

			assertion.ExtensionOutputs = new(webauthn.GetAuthenticationExtensionsClientOutputs)

			// Yield assertions without extension data
			if !assertion.AuthData.Flags.ExtensionDataIncluded() {
				if !yield(assertion, nil) {
					return
				}
				continue
			}

			// credBlob
			if assertion.AuthData.Extensions.GetCredBlobOutput != nil {
				assertion.ExtensionOutputs.GetCredentialBlobOutputs = &webauthn.GetCredentialBlobOutputs{
					GetCredBlob: assertion.AuthData.Extensions.CredBlob,
				}
			}

			// hmac-secret or prf
			if assertion.AuthData.Extensions.GetHMACSecretOutput != nil {
				salt, err := protocol.Decrypt(sharedSecret, assertion.AuthData.Extensions.HMACSecret)
				if err != nil {
					yield(nil, err)
					return
				}

				switch len(salt) {
				case 32:
					if extInputs.GetHMACSecretInputs != nil {
						assertion.ExtensionOutputs.GetHMACSecretOutputs = &webauthn.GetHMACSecretOutputs{
							HMACGetSecret: webauthn.HMACGetSecretOutput{
								Output1: salt[:32],
							},
						}
					}
					if extInputs.PRFInputs != nil {
						assertion.ExtensionOutputs.PRFOutputs = &webauthn.PRFOutputs{
							PRF: webauthn.AuthenticationExtensionsPRFOutputs{
								Enabled: true,
								Results: webauthn.AuthenticationExtensionsPRFValues{
									First: salt[:32],
								},
							},
						}
					}
				case 64:
					if extInputs.GetHMACSecretInputs != nil {
						assertion.ExtensionOutputs.GetHMACSecretOutputs = &webauthn.GetHMACSecretOutputs{
							HMACGetSecret: webauthn.HMACGetSecretOutput{
								Output1: salt[:32],
								Output2: salt[32:],
							},
						}
					}
					if extInputs.PRFInputs != nil {
						assertion.ExtensionOutputs.PRFOutputs = &webauthn.PRFOutputs{
							PRF: webauthn.AuthenticationExtensionsPRFOutputs{
								Enabled: true,
								Results: webauthn.AuthenticationExtensionsPRFValues{
									First:  salt[:32],
									Second: salt[32:],
								},
							},
						}
					}
				default:
					yield(nil, newErrorMessage(ErrInvalidSaltSize, "salt must be 32 or 64 bytes"))
					return
				}
			}

			if !yield(assertion, nil) {
				return
			}
		}
	}
}

// GetPinUvAuthTokenUsingPIN obtains a pinUvAuthToken using a given PIN.
func (d *Device) GetPinUvAuthTokenUsingPIN(
	pin string,
	permissions ctap2.Permission,
	rpID string,
) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	noMcGaPermission, ok := d.info.Options[ctap2.OptionNoMcGaPermissionsWithClientPin]
	if ok && noMcGaPermission &&
		(permissions&ctap2.PermissionMakeCredential != 0 || permissions&ctap2.PermissionGetAssertion != 0) {
		return nil, newErrorMessage(
			ErrNotSupported,
			"you cannot get a pinUvAuthToken using PIN with MakeCredential or GetAssertion permissions if device has noMcGaPermissionsWithClientPin option",
		)
	}

	clientPIN, ok := d.info.Options[ctap2.OptionClientPIN]
	if !ok {
		return nil, newErrorMessage(
			ErrNotSupported,
			"you cannot get a pinUvAuthToken using PIN if device hasn't clientPin option",
		)
	}
	if !clientPIN {
		return nil, newErrorMessage(
			ErrPinNotSet,
			"please set PIN first",
		)
	}

	if _, ok := d.info.Options[ctap2.OptionBioEnroll]; !ok && permissions&ctap2.PermissionBioEnrollment != 0 {
		return nil, newErrorMessage(
			ErrNotSupported,
			"you cannot set be BioEnrollment permission if device doesn't support bioEnroll option",
		)
	}

	authnrCfg, ok := d.info.Options[ctap2.OptionAuthenticatorConfig]
	if (!ok || !authnrCfg) && permissions&ctap2.PermissionAuthenticatorConfiguration != 0 {
		return nil, newErrorMessage(
			ErrNotSupported,
			"you cannot set be AuthenticatorConfiguration permission if device doesn't support uv option")
	}

	pinProtocol, err := d.pinUvAuthProtocol()
	if err != nil {
		return nil, err
	}

	keyAgreement, err := d.ctapClient.GetKeyAgreement(pinProtocol)
	if err != nil {
		return nil, err
	}

	token, ok := d.info.Options[ctap2.OptionPinUvAuthToken]
	if !ok || !token {
		return d.ctapClient.GetPinToken(
			pinProtocol,
			keyAgreement,
			pin,
		)
	}

	return d.ctapClient.GetPinUvAuthTokenUsingPinWithPermissions(
		pinProtocol,
		keyAgreement,
		pin,
		permissions,
		rpID,
	)
}
