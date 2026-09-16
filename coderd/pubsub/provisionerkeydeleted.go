package pubsub

import "github.com/google/uuid"

// ProvisionerKeyDeletedChannel returns the pubsub channel that carries a
// notification when the provisioner key with the given ID is deleted.
func ProvisionerKeyDeletedChannel(keyID uuid.UUID) string {
	return "provisioner_key_deleted:" + keyID.String()
}
