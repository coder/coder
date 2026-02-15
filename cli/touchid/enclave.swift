// Secure Enclave operations using CryptoKit.
// Compiled separately by swiftc and linked into the Go binary via CGO.
// CryptoKit's SecureEnclave APIs work without code signing entitlements,
// unlike the Security framework's SecKeyCreateRandomKey path.

import Foundation
import CryptoKit
import LocalAuthentication

// MARK: - C-exported functions

/// Check if Secure Enclave is available on this device.
@_cdecl("swift_se_available")
public func swiftSEAvailable() -> Int32 {
    return SecureEnclave.isAvailable ? 1 : 0
}

/// Check if biometrics (Touch ID / Face ID) are available.
@_cdecl("swift_bio_available")
public func swiftBioAvailable() -> Int32 {
    let ctx = LAContext()
    var error: NSError?
    let ok = ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &error)
    return ok ? 1 : 0
}

/// Generate a new Secure Enclave P-256 signing key.
/// Returns:
///   - public_key_out: base64 of 65-byte x963 public key (0x04 || X || Y)
///   - data_rep_out: base64 of the encrypted dataRepresentation (for persistence)
///   - error_out: error message, or NULL on success
/// Caller must free all returned strings with free().
@_cdecl("swift_se_generate")
public func swiftSEGenerate(
    _ publicKeyOut: UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>,
    _ dataRepOut: UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>,
    _ errorOut: UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>
) -> Int32 {
    do {
        // Create access control that requires biometry (Touch ID)
        // for every private key usage (signing). Without this, the
        // key can be used without any user interaction, which
        // defeats the security purpose.
        var accessError: Unmanaged<CFError>?
        guard let accessControl = SecAccessControlCreateWithFlags(
            kCFAllocatorDefault,
            kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
            [.privateKeyUsage, .biometryAny],
            &accessError
        ) else {
            let desc = accessError?.takeRetainedValue().localizedDescription ?? "unknown"
            errorOut.pointee = strdup("failed to create access control: \(desc)")
            return -1
        }

        let privateKey = try SecureEnclave.P256.Signing.PrivateKey(
            accessControl: accessControl
        )

        let pubKeyData = privateKey.publicKey.x963Representation
        let dataRep = privateKey.dataRepresentation

        publicKeyOut.pointee = strdup(pubKeyData.base64EncodedString())
        dataRepOut.pointee = strdup(dataRep.base64EncodedString())
        return 0
    } catch {
        errorOut.pointee = strdup(error.localizedDescription)
        return -1
    }
}

/// Sign data using a Secure Enclave key restored from its dataRepresentation.
/// This triggers a Touch ID prompt.
/// Parameters:
///   - data_rep_b64: base64-encoded dataRepresentation of the private key
///   - message_b64: base64-encoded message bytes to sign
/// Returns:
///   - sig_out: base64-encoded DER signature
///   - error_out: error message, or NULL on success
/// Caller must free returned strings with free().
@_cdecl("swift_se_sign")
public func swiftSESign(
    _ dataRepB64: UnsafePointer<CChar>,
    _ messageB64: UnsafePointer<CChar>,
    _ sigOut: UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>,
    _ errorOut: UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>
) -> Int32 {
    guard let dataRepData = Data(base64Encoded: String(cString: dataRepB64)) else {
        errorOut.pointee = strdup("invalid base64 dataRepresentation")
        return -1
    }
    guard let messageData = Data(base64Encoded: String(cString: messageB64)) else {
        errorOut.pointee = strdup("invalid base64 message")
        return -1
    }

    // Require biometric authentication BEFORE signing. This call
    // shows the macOS Touch ID dialog. The signing only proceeds
    // if the user authenticates. Without this gate, the Secure
    // Enclave would sign silently from CLI processes.
    let context = LAContext()
    // Disable fallback to password — we specifically want biometrics.
    context.localizedFallbackTitle = ""

    let semaphore = DispatchSemaphore(value: 0)
    var authSuccess = false
    var authError: Error?

    context.evaluatePolicy(
        .deviceOwnerAuthenticationWithBiometrics,
        localizedReason: "Coder needs your fingerprint for a secure workspace connection"
    ) { success, error in
        authSuccess = success
        authError = error
        semaphore.signal()
    }

    // Wait for Touch ID with a generous timeout.
    let waitResult = semaphore.wait(timeout: .now() + 60)
    if waitResult == .timedOut {
        errorOut.pointee = strdup("Touch ID prompt timed out")
        return -1
    }

    if !authSuccess {
        if let laErr = authError as? LAError,
           laErr.code == .userCancel || laErr.code == .appCancel {
            errorOut.pointee = strdup("user cancelled biometric authentication")
            return 2
        }
        let desc = authError?.localizedDescription ?? "biometric authentication failed"
        errorOut.pointee = strdup(desc)
        return -1
    }

    // Biometric check passed — now sign with the authenticated context.
    do {
        let privateKey = try SecureEnclave.P256.Signing.PrivateKey(
            dataRepresentation: dataRepData,
            authenticationContext: context
        )

        let signature = try privateKey.signature(for: messageData)
        let derSig = signature.derRepresentation

        sigOut.pointee = strdup(derSig.base64EncodedString())
        return 0
    } catch let error as NSError {
        errorOut.pointee = strdup(error.localizedDescription)
        if error.domain == LAError.errorDomain &&
           error.code == LAError.userCancel.rawValue {
            return 2
        }
        return -1
    } catch {
        errorOut.pointee = strdup(error.localizedDescription)
        return -1
    }
}
