package pubsub

import "github.com/google/uuid"

// ProvisionerKeyDeletedChannel returns the pubsub channel that carries a
// notification when the provisioner key with the given ID is deleted.
//
// Provisioner daemon serve sessions authenticated with a key subscribe to
// their own key's channel and tear down the session on receipt, because auth
// is only checked at connection establishment. The payload is empty: the
// channel encodes the key ID, and a missed message is backstopped by a
// key-existence check on job acquisition.
func ProvisionerKeyDeletedChannel(keyID uuid.UUID) string {
	return "provisioner_key_deleted:" + keyID.String()
}
