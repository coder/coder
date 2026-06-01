package proto

import (
	"bytes"
	"crypto/sha256"
	"sync"

	"golang.org/x/xerrors"
)

const (
	ChunkSize   = 2 << 20         // 2 MiB
	MaxFileSize = 10 * (10 << 20) // 100 MiB, matches coderd HTTPFileMaxBytes
)

type DataBuilder struct {
	Type       DataUploadType
	Hash       []byte
	Size       int64
	ChunkCount int32

	// chunkIndex is the index of the next chunk to be added.
	chunkIndex int32
	mu         sync.Mutex
	data       []byte
}

func NewDataBuilder(req *DataUpload) (*DataBuilder, error) {
	if len(req.DataHash) != 32 {
		return nil, xerrors.Errorf("data hash must be 32 bytes, got %d bytes", len(req.DataHash))
	}

	if req.FileSize < 0 {
		return nil, xerrors.Errorf("file size must not be negative, got %d", req.FileSize)
	}
	if req.FileSize > MaxFileSize {
		return nil, xerrors.Errorf("file size %d exceeds maximum allowed %d", req.FileSize, MaxFileSize)
	}
	if req.Chunks < 0 {
		return nil, xerrors.Errorf("chunk count must not be negative, got %d", req.Chunks)
	}
	//nolint:gosec // FileSize is validated to be <= MaxFileSize, well within int32 range
	maxChunks := int32((req.FileSize + ChunkSize - 1) / ChunkSize)
	if req.Chunks > maxChunks {
		return nil, xerrors.Errorf("chunk count %d exceeds maximum %d for file size %d", req.Chunks, maxChunks, req.FileSize)
	}

	return &DataBuilder{
		Type:       req.UploadType,
		Hash:       req.DataHash,
		Size:       req.FileSize,
		ChunkCount: req.Chunks,

		// Initial conditions
		chunkIndex: 0,
		data:       make([]byte, 0, req.FileSize),
	}, nil
}

func (b *DataBuilder) Add(chunk *ChunkPiece) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !bytes.Equal(b.Hash, chunk.FullDataHash) {
		return b.done(), xerrors.Errorf("data hash does not match, this chunk is for a different data upload")
	}

	if b.done() {
		return b.done(), xerrors.Errorf("data upload is already complete, cannot add more chunks")
	}

	if chunk.PieceIndex != b.chunkIndex {
		return b.done(), xerrors.Errorf("chunks ordering, expected chunk index %d, got %d", b.chunkIndex, chunk.PieceIndex)
	}

	expectedSize := len(b.data) + len(chunk.Data)
	if expectedSize > int(b.Size) {
		return b.done(), xerrors.Errorf("data exceeds expected size, data is now %d bytes, %d bytes over the limit of %d",
			expectedSize, int64(expectedSize)-b.Size, b.Size)
	}

	b.data = append(b.data, chunk.Data...)
	b.chunkIndex++

	return b.done(), nil
}

// IsDone is always safe to call
func (b *DataBuilder) IsDone() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.done()
}

func (b *DataBuilder) Complete() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.done() {
		return nil, xerrors.Errorf("data upload is not complete, expected %d chunks, got %d", b.ChunkCount, b.chunkIndex)
	}

	if len(b.data) != int(b.Size) {
		return nil, xerrors.Errorf("data size mismatch, expected %d bytes, got %d bytes", b.Size, len(b.data))
	}

	hash := sha256.Sum256(b.data)
	if !bytes.Equal(hash[:], b.Hash) {
		return nil, xerrors.Errorf("data hash mismatch, expected %x, got %x", b.Hash, hash[:])
	}

	// A safe method would be to return a copy of the data, but that would have to
	// allocate double the memory. Just return the original slice, and let the caller
	// handle the memory management.
	return b.data, nil
}

func (b *DataBuilder) done() bool {
	return b.chunkIndex >= b.ChunkCount
}

func BytesToDataUpload(dataType DataUploadType, data []byte) (*DataUpload, []*ChunkPiece, error) {
	if int64(len(data)) > MaxFileSize {
		return nil, nil, xerrors.Errorf("data size %d exceeds maximum allowed %d", len(data), MaxFileSize)
	}

	fullHash := sha256.Sum256(data)
	//nolint:gosec // not going over int32
	size := int32(len(data))
	// basically ceiling division to get the number of chunks required to
	// hold the data, each chunk is ChunkSize bytes.
	chunkCount := (size + ChunkSize - 1) / ChunkSize

	req := &DataUpload{
		DataHash:   fullHash[:],
		FileSize:   int64(size),
		Chunks:     chunkCount,
		UploadType: dataType,
	}

	chunks := make([]*ChunkPiece, 0, chunkCount)
	for i := int32(0); i < chunkCount; i++ {
		start := int64(i) * ChunkSize
		end := start + ChunkSize
		if end > int64(size) {
			end = int64(size)
		}
		chunkData := data[start:end]

		chunk := &ChunkPiece{
			PieceIndex:   i,
			Data:         chunkData,
			FullDataHash: fullHash[:],
		}
		chunks = append(chunks, chunk)
	}

	return req, chunks, nil
}
