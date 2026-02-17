-- Add connect_public_key column to api_keys for Secure Enclave
-- connect auth. When set, workspace connections (SSH, port-forward)
-- require an ECDSA P-256 signature proof from the client's Secure
-- Enclave. The column stores the raw 65-byte uncompressed EC point
-- (0x04 || X || Y).
ALTER TABLE api_keys ADD COLUMN connect_public_key bytea;
