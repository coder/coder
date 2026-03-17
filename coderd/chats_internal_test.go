package coderd

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatImageResizedDimensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		w, h        int
		maxDim      int
		wantW       int
		wantH       int
		wantResized bool
	}{
		{"small image", 100, 100, 2000, 100, 100, false},
		{"wide landscape", 4000, 2000, 2000, 2000, 1000, true},
		{"tall portrait", 1500, 3000, 2000, 1000, 2000, true},
		{"square", 3000, 3000, 2000, 2000, 2000, true},
		{"exactly at limit", 2000, 1500, 2000, 2000, 1500, false},
		{"one at limit other below", 2000, 500, 2000, 2000, 500, false},
		{"custom max", 1000, 800, 500, 500, 400, true},
		{"very thin vertical", 1, 5000, 2000, 1, 2000, true},
		{"very thin horizontal", 5000, 1, 2000, 2000, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH, gotResized := chatImageResizedDimensions(tt.w, tt.h, tt.maxDim)
			assert.Equal(t, tt.wantW, gotW, "width")
			assert.Equal(t, tt.wantH, gotH, "height")
			assert.Equal(t, tt.wantResized, gotResized, "resized")
		})
	}
}

func TestResizeChatImage(t *testing.T) {
	t.Parallel()

	makePNG := func(t *testing.T, w, h int) []byte {
		t.Helper()
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		var buf bytes.Buffer
		require.NoError(t, png.Encode(&buf, img))
		return buf.Bytes()
	}

	makeJPEG := func(t *testing.T, w, h int) []byte {
		t.Helper()
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		var buf bytes.Buffer
		require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
		return buf.Bytes()
	}

	t.Run("SmallPNG/NoResize", func(t *testing.T) {
		t.Parallel()
		data := makePNG(t, 800, 600)
		out, mime := resizeChatImage(data, "image/png")
		assert.Equal(t, "image/png", mime)
		assert.Equal(t, data, out)
	})

	t.Run("OversizedPNG/Resized", func(t *testing.T) {
		t.Parallel()
		data := makePNG(t, 4000, 2000)
		out, mime := resizeChatImage(data, "image/png")
		assert.Equal(t, "image/png", mime)
		img, _, err := image.Decode(bytes.NewReader(out))
		require.NoError(t, err)
		bounds := img.Bounds()
		assert.Equal(t, 2000, bounds.Dx())
		assert.Equal(t, 1000, bounds.Dy())
	})

	t.Run("OversizedJPEG/StaysJPEG", func(t *testing.T) {
		t.Parallel()
		data := makeJPEG(t, 3000, 3000)
		out, mime := resizeChatImage(data, "image/jpeg")
		assert.Equal(t, "image/jpeg", mime)
		img, _, err := image.Decode(bytes.NewReader(out))
		require.NoError(t, err)
		bounds := img.Bounds()
		assert.Equal(t, 2000, bounds.Dx())
		assert.Equal(t, 2000, bounds.Dy())
	})

	t.Run("InvalidData/PassThrough", func(t *testing.T) {
		t.Parallel()
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		out, mime := resizeChatImage(data, "image/png")
		assert.Equal(t, "image/png", mime)
		assert.Equal(t, data, out)
	})

	t.Run("NonImageMime/PassThrough", func(t *testing.T) {
		t.Parallel()
		data := []byte("not an image")
		out, mime := resizeChatImage(data, "text/plain")
		assert.Equal(t, "text/plain", mime)
		assert.Equal(t, data, out)
	})
}
