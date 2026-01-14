package metadatabatcher

import (
	"encoding/base64"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	// uuidBase64Size is the size of a base64-encoded UUID without padding (22 characters).
	uuidBase64Size = 22

	// maxAgentIDsPerChunk is the maximum number of agent IDs that can fit in a
	// single pubsub message. PostgreSQL NOTIFY has an 8KB limit.
	// With base64 encoding, each UUID is 22 characters, so we can fit
	// ~363 agent IDs per chunk (8000 / 22 = 363.6).
	maxAgentIDsPerChunk = 363
)

// EncodeAgentIDChunks encodes agent IDs into chunks that fit within the
// PostgreSQL NOTIFY 8KB payload size limit. Each UUID is base64-encoded
// (without padding) and concatenated into a single byte slice per chunk.
func EncodeAgentIDChunks(agentIDs []uuid.UUID) [][]byte {
	chunks := make([][]byte, 0, (len(agentIDs)+maxAgentIDsPerChunk-1)/maxAgentIDsPerChunk)

	for i := 0; i < len(agentIDs); i += maxAgentIDsPerChunk {
		end := i + maxAgentIDsPerChunk
		if end > len(agentIDs) {
			end = len(agentIDs)
		}

		chunk := agentIDs[i:end]

		// Build payload by base64-encoding each UUID (without padding) and
		// concatenating them. This is UTF-8 safe for PostgreSQL NOTIFY.
		payload := make([]byte, 0, len(chunk)*uuidBase64Size)
		for _, agentID := range chunk {
			// Encode UUID bytes to base64 without padding (RawStdEncoding).
			// This produces exactly 22 characters per UUID.
			encoded := base64.RawStdEncoding.AppendEncode(payload, agentID[:])
			payload = encoded
		}

		chunks = append(chunks, payload)
	}

	return chunks
}

// AgentIDIterator provides lazy decoding of base64-encoded agent IDs
// from a byte slice. Each agent ID is decoded on demand as Next() is called.
type AgentIDIterator struct {
	data   []byte
	offset int
	err    error
}

// NewAgentIDIterator creates an iterator for lazily decoding agent IDs
// from a base64-encoded byte slice. Returns an error if the data size
// is not a multiple of uuidBase64Size (22 bytes).
func NewAgentIDIterator(data []byte) (*AgentIDIterator, error) {
	if len(data)%uuidBase64Size != 0 {
		return nil, xerrors.Errorf("invalid data size %d, must be multiple of %d", len(data), uuidBase64Size)
	}

	return &AgentIDIterator{
		data:   data,
		offset: 0,
	}, nil
}

// Next decodes and returns the next agent ID. Returns false when there
// are no more IDs to decode. If decoding fails, Next() returns false
// and Err() will return the error.
func (it *AgentIDIterator) Next() (uuid.UUID, bool) {
	if it.offset >= len(it.data) {
		return uuid.UUID{}, false
	}

	// Decode the 22-character base64 string back to 16 bytes.
	var agentIDBytes [16]byte
	n, err := base64.RawStdEncoding.Decode(agentIDBytes[:], it.data[it.offset:it.offset+uuidBase64Size])
	if err != nil || n != 16 {
		if err == nil {
			err = xerrors.Errorf("decoded %d bytes, expected 16", n)
		}
		it.err = xerrors.Errorf("failed to decode agent ID at offset %d: %w", it.offset, err)
		return uuid.UUID{}, false
	}

	agentID, err := uuid.FromBytes(agentIDBytes[:])
	if err != nil {
		it.err = xerrors.Errorf("invalid agent ID bytes at offset %d: %w", it.offset, err)
		return uuid.UUID{}, false
	}

	it.offset += uuidBase64Size
	return agentID, true
}

// Err returns any error encountered during iteration.
func (it *AgentIDIterator) Err() error {
	return it.err
}

// Count returns the total number of agent IDs in the data without
// decoding them.
func (it *AgentIDIterator) Count() int {
	return len(it.data) / uuidBase64Size
}
