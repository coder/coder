#ifndef ENCLAVE_H_
#define ENCLAVE_H_

// These functions are implemented in enclave.swift and compiled
// separately via swiftc. They use CryptoKit's SecureEnclave APIs
// which work without code signing entitlements.

// Check Secure Enclave availability. Returns 1 if available.
int swift_se_available(void);

// Check biometric availability. Returns 1 if available.
int swift_bio_available(void);

// Generate a new Secure Enclave P-256 signing key.
// On success (return 0): public_key_out and data_rep_out are set
// to base64 strings (caller must free).
// On error (return -1): error_out is set (caller must free).
int swift_se_generate(
    char **public_key_out,
    char **data_rep_out,
    char **error_out);

// Sign message bytes using a Secure Enclave key.
// Triggers Touch ID prompt with the given reason string.
// data_rep_b64: base64 dataRepresentation of the private key.
// message_b64: base64 message to sign.
// reason: text shown in the Touch ID dialog.
// On success (return 0): sig_out is base64 DER signature.
// On error (return -1): error_out is set.
// On user cancel (return 2): error_out is set.
int swift_se_sign(
    const char *data_rep_b64,
    const char *message_b64,
    const char *reason,
    char **sig_out,
    char **error_out);

#endif /* ENCLAVE_H_ */
