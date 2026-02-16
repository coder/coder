//go:build darwin && cgo

package touchid

// Link the pre-compiled Swift static library (built by build_swift.sh)
// and the Swift runtime + Apple frameworks it depends on.

// #cgo CFLAGS: -Wall
// #cgo LDFLAGS: -L${SRCDIR} -lenclave
// #cgo LDFLAGS: -L/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/lib/swift/macosx
// #cgo LDFLAGS: -Wl,-rpath,/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/lib/swift/macosx
// #cgo LDFLAGS: -Wl,-rpath,/usr/lib/swift
// #cgo LDFLAGS: -lswiftCore
// #cgo LDFLAGS: -framework Foundation -framework Security -framework LocalAuthentication -framework CryptoKit
// #include <stdlib.h>
// #include "enclave.h"
import "C"

import (
	"fmt"
	"unsafe"
)

// IsAvailable returns true when Secure Enclave and biometrics are
// both present on this Mac.
func IsAvailable() bool {
	return C.swift_se_available() != 0 && C.swift_bio_available() != 0
}

// GenerateKey creates a new ECDSA P-256 keypair in the Secure
// Enclave. Returns the base64-encoded 65-byte x963 public key and
// the base64-encoded dataRepresentation (for persistence).
// The private key stays in hardware and never leaves.
func GenerateKey() (publicKeyB64 string, dataRepB64 string, err error) {
	var cPubKey *C.char
	var cDataRep *C.char
	var cErr *C.char

	status := C.swift_se_generate(&cPubKey, &cDataRep, &cErr)
	defer func() {
		if cPubKey != nil {
			C.free(unsafe.Pointer(cPubKey))
		}
		if cDataRep != nil {
			C.free(unsafe.Pointer(cDataRep))
		}
		if cErr != nil {
			C.free(unsafe.Pointer(cErr))
		}
	}()

	if status != 0 {
		msg := "unknown error"
		if cErr != nil {
			msg = C.GoString(cErr)
		}
		return "", "", fmt.Errorf("touchid generate: %s", msg)
	}
	return C.GoString(cPubKey), C.GoString(cDataRep), nil
}

// Sign signs the given base64-encoded message using the Secure
// Enclave key identified by its dataRepresentation. This triggers
// a Touch ID prompt. Returns the base64-encoded DER ECDSA signature.
func Sign(dataRepB64, messageB64, reason string) (string, error) {
	cDataRep := C.CString(dataRepB64)
	cMsg := C.CString(messageB64)
	cReason := C.CString(reason)
	defer C.free(unsafe.Pointer(cDataRep))
	defer C.free(unsafe.Pointer(cMsg))
	defer C.free(unsafe.Pointer(cReason))

	var cSig *C.char
	var cErr *C.char

	status := C.swift_se_sign(cDataRep, cMsg, cReason, &cSig, &cErr)
	defer func() {
		if cSig != nil {
			C.free(unsafe.Pointer(cSig))
		}
		if cErr != nil {
			C.free(unsafe.Pointer(cErr))
		}
	}()

	if status == 2 {
		return "", ErrUserCancelled
	}
	if status != 0 {
		msg := "unknown error"
		if cErr != nil {
			msg = C.GoString(cErr)
		}
		return "", fmt.Errorf("touchid sign: %s", msg)
	}
	return C.GoString(cSig), nil
}

// DeleteKey is a no-op for CryptoKit-based keys. The key is
// "deleted" by removing the dataRepresentation file, which is
// handled by the caller.
func DeleteKey(_ string) error {
	return nil
}

// Diagnostic checks Secure Enclave and biometric availability.
func Diagnostic() (available, hasSecureEnclave, hasBiometrics bool, err error) {
	se := C.swift_se_available() != 0
	bio := C.swift_bio_available() != 0
	return se && bio, se, bio, nil
}
