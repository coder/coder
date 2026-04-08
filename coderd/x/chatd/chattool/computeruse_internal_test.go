package chattool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeScaledScreenshotSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		wantW, wantH  int
	}{
		{
			name:   "1920x1080_prefers_standard_1280x720",
			width:  1920,
			height: 1080,
			wantW:  1280,
			wantH:  720,
		},
		{
			name:   "1280x800_no_scaling",
			width:  1280,
			height: 800,
			wantW:  1280,
			wantH:  800,
		},
		{
			name:   "3840x2160_prefers_standard_1280x720",
			width:  3840,
			height: 2160,
			wantW:  1280,
			wantH:  720,
		},
		{
			name:   "1568x1000_prefers_standard_1280x816",
			width:  1568,
			height: 1000,
			wantW:  1280,
			wantH:  816,
		},
		{
			name:   "100x100_small_display",
			width:  100,
			height: 100,
			wantW:  100,
			wantH:  100,
		},
		{
			name:   "4000x3000_prefers_standard_1024x768",
			width:  4000,
			height: 3000,
			wantW:  1024,
			wantH:  768,
		},
		{
			name:   "1920x1200_prefers_standard_1280x800",
			width:  1920,
			height: 1200,
			wantW:  1280,
			wantH:  800,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := computeScaledScreenshotSize(tt.width, tt.height)
			assert.Equal(t, tt.wantW, gotW)
			assert.Equal(t, tt.wantH, gotH)

			// Invariant: results must respect Anthropic constraints.
			const maxLongEdge = 1568
			const maxTotalPixels = 1_150_000
			longEdge := max(gotW, gotH)
			assert.LessOrEqual(t, longEdge, maxLongEdge,
				"long edge %d exceeds max %d", longEdge, maxLongEdge)
			assert.LessOrEqual(t, gotW*gotH, maxTotalPixels,
				"total pixels %d exceeds max %d", gotW*gotH, maxTotalPixels)
		})
	}
}
