package metadatabatcher

import (
	"encoding/base64"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	// uuidBase64Size is the size of a base64-encoded UUID without padding (22 characters).
	UUIDBase64Size = 22

	// maxAgentIDsPerChunk is the maximum number of agent IDs that can fit in a
	// single pubsub message. PostgreSQL NOTIFY has an 8KB limit.
	// With base64 encoding, each UUID is 22 characters, so we can fit
	// ~363 agent IDs per chunk (8000 / 22 = 363.6).
	maxAgentIDsPerChunk = maxPubsubPayloadSize / UUIDBase64Size
)

func EncodeAgentID(agentID uuid.UUID, dst []byte) error {
	// Encode UUID bytes to base64 without padding (RawStdEncoding).
	// This produces exactly 22 characters per UUID.
	reqLen := base64.RawStdEncoding.EncodedLen(len(agentID))
	if len(dst) < reqLen {
		return xerrors.Errorf("destination byte slice was too small %d, required %d", len(dst), reqLen)
	}
	base64.RawStdEncoding.Encode(dst, agentID[:])
	return nil
}

// EncodeAgentIDChunks encodes agent IDs into chunks that fit within the
// PostgreSQL NOTIFY 8KB payload size limit. Each UUID is base64-encoded
// (without padding) and concatenated into a single byte slice per chunk.
func EncodeAgentIDChunks(agentIDs []uuid.UUID) ([][]byte, error) {
	chunks := make([][]byte, 0, (len(agentIDs)+maxAgentIDsPerChunk-1)/maxAgentIDsPerChunk)

	for i := 0; i < len(agentIDs); i += maxAgentIDsPerChunk {
		end := i + maxAgentIDsPerChunk
		if end > len(agentIDs) {
			end = len(agentIDs)
		}

		chunk := agentIDs[i:end]

		// Build payload by base64-encoding each UUID (without padding) and
		// concatenating them. This is UTF-8 safe for PostgreSQL NOTIFY.
		payload := make([]byte, len(chunk)*UUIDBase64Size)
		for i, agentID := range chunk {
			err := EncodeAgentID(agentID, payload[i*UUIDBase64Size:(i+1)*UUIDBase64Size])
			if err != nil {
				return nil, err
			}
		}
		chunks = append(chunks, payload)
	}

	return chunks, nil
}
