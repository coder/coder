package provisionersdk_test

import (
	crand "crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/codersdk/drpcsdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// TestProtobufMessageSizeDetection tests that we can detect when a protobuf message
// would exceed the maximum size limit and needs streaming
func TestProtobufMessageSizeDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		moduleFileSize int
		expectLarge    bool
	}{
		{
			name:           "small_module_files",
			moduleFileSize: 1024, // 1KB - should not exceed limit
			expectLarge:    false,
		},
		{
			name:           "large_module_files",
			moduleFileSize: drpcsdk.MaxMessageSize + 1024, // Over 4MB - should exceed limit
			expectLarge:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate module files data
			moduleFiles := make([]byte, tc.moduleFileSize)
			_, err := crand.Read(moduleFiles)
			require.NoError(t, err)

			// Create plan response with module files
			planComplete := &sdkproto.PlanComplete{
				ModuleFiles: moduleFiles,
				Resources:   []*sdkproto.Resource{}, // Empty for simplicity
			}

			// Create response wrapper
			resp := &sdkproto.Response{
				Type: &sdkproto.Response_Plan{Plan: planComplete},
			}

			// Check if the message size exceeds the limit
			messageSize := proto.Size(resp)
			exceedsLimit := messageSize > drpcsdk.MaxMessageSize

			require.Equal(t, tc.expectLarge, exceedsLimit,
				"message size %d bytes, limit %d bytes", messageSize, drpcsdk.MaxMessageSize)

			if exceedsLimit {
				// Verify that streaming would work
				_, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, moduleFiles)
				require.Greater(t, len(chunks), 0, "should generate chunks for streaming")

				// Verify that plan without module files is small enough
				planWithoutModules := &sdkproto.PlanComplete{
					ModuleFiles: nil, // Empty for streaming
					Resources:   planComplete.Resources,
				}
				respWithoutModules := &sdkproto.Response{
					Type: &sdkproto.Response_Plan{Plan: planWithoutModules},
				}
				smallerSize := proto.Size(respWithoutModules)
				require.Less(t, smallerSize, drpcsdk.MaxMessageSize,
					"plan without modules should be under limit")
			}
		})
	}
}

// TestStreamingFallbackLogic tests the logic for when to use streaming vs inline
func TestStreamingFallbackLogic(t *testing.T) {
	t.Parallel()

	// Test with various sizes that would trigger streaming
	testSizes := []int{
		drpcsdk.MaxMessageSize + 1,
		drpcsdk.MaxMessageSize * 2,
		drpcsdk.MaxMessageSize + sdkproto.ChunkSize/2,
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			// Generate test data
			originalData := make([]byte, size)
			_, err := crand.Read(originalData)
			require.NoError(t, err)

			// Test that BytesToDataUpload works correctly
			upload, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, originalData)
			require.Greater(t, len(chunks), 0, "should generate chunks for large data")

			// Test that data can be reassembled
			builder, err := sdkproto.NewDataBuilder(upload)
			require.NoError(t, err)

			for _, chunk := range chunks {
				_, err := builder.Add(chunk)
				require.NoError(t, err)
			}

			reassembledData, err := builder.Complete()
			require.NoError(t, err)

			// Verify data integrity
			require.Equal(t, originalData, reassembledData, "reassembled data should match original")
		})
	}
}
