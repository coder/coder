-- Superseded by credential_lifecycle_ledger. The table was built to get one
-- end to end cycle working, and three of its decisions did not survive: a
-- credential needs an identity, since a journal subject must be nameable;
-- revocation updates a state rather than deleting a row, because a ledger keeps
-- its retired rows; and a table restricted to what is currently valid is the
-- shape that decision rejects.
DROP TABLE valid_credentials;
