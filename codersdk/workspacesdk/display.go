package workspacesdk

import "math"

const (
	// DesktopNativeWidth is the default native desktop width in pixels used for
	// computer-use desktop sessions.
	DesktopNativeWidth = 1920
	// DesktopNativeHeight is the default native desktop height in pixels used for
	// computer-use desktop sessions.
	DesktopNativeHeight = 1080

	desktopDeclaredMaxLongEdge    = 1568
	desktopDeclaredMaxTotalPixels = 1_150_000

	// OpenAI recommends 1440x900 or 1600x900 for computer use.
	// Use 1600x900 so screenshots keep the native 16:9 aspect ratio.
	desktopOpenAIComputerUseDeclaredWidth  = 1600
	desktopOpenAIComputerUseDeclaredHeight = 900
)

var preferredDeclaredDesktopWidths = []int{1280, 1024}

// DesktopGeometry describes the native workspace desktop and the declared
// model-facing geometry used for screenshots and coordinates.
type DesktopGeometry struct {
	NativeWidth    int
	NativeHeight   int
	DeclaredWidth  int
	DeclaredHeight int
}

// DefaultDesktopGeometry returns the default native desktop geometry together
// with the declared model-facing geometry derived from it.
func DefaultDesktopGeometry() DesktopGeometry {
	return NewDesktopGeometry(DesktopNativeWidth, DesktopNativeHeight)
}

// DefaultOpenAIComputerUseDesktopGeometry returns the default native desktop
// geometry with OpenAI's recommended computer-use declared dimensions.
func DefaultOpenAIComputerUseDesktopGeometry() DesktopGeometry {
	return NewDesktopGeometryWithDeclared(
		DesktopNativeWidth,
		DesktopNativeHeight,
		desktopOpenAIComputerUseDeclaredWidth,
		desktopOpenAIComputerUseDeclaredHeight,
	)
}

// NewDesktopGeometry derives a declared model-facing geometry from the native
// desktop size.
func NewDesktopGeometry(nativeWidth, nativeHeight int) DesktopGeometry {
	nativeWidth = sanitizeDesktopDimension(nativeWidth)
	nativeHeight = sanitizeDesktopDimension(nativeHeight)

	declaredWidth, declaredHeight := computeDeclaredDesktopSize(
		nativeWidth,
		nativeHeight,
	)

	return DesktopGeometry{
		NativeWidth:    nativeWidth,
		NativeHeight:   nativeHeight,
		DeclaredWidth:  declaredWidth,
		DeclaredHeight: declaredHeight,
	}
}

// NewDesktopGeometryWithDeclared returns a geometry that preserves the native
// desktop size while using the provided declared model-facing dimensions.
func NewDesktopGeometryWithDeclared(
	nativeWidth,
	nativeHeight,
	declaredWidth,
	declaredHeight int,
) DesktopGeometry {
	nativeWidth = sanitizeDesktopDimension(nativeWidth)
	nativeHeight = sanitizeDesktopDimension(nativeHeight)
	if declaredWidth <= 0 {
		declaredWidth = nativeWidth
	}
	if declaredHeight <= 0 {
		declaredHeight = nativeHeight
	}

	return DesktopGeometry{
		NativeWidth:    nativeWidth,
		NativeHeight:   nativeHeight,
		DeclaredWidth:  sanitizeDesktopDimension(declaredWidth),
		DeclaredHeight: sanitizeDesktopDimension(declaredHeight),
	}
}

// DeclaredPointToNative maps a point from declared model-facing coordinates to
// native desktop coordinates using the existing pixel-center truncation rule.
func (g DesktopGeometry) DeclaredPointToNative(x, y int) (nativeX, nativeY int) {
	return scaleDesktopCoordinate(x, g.DeclaredWidth, g.NativeWidth),
		scaleDesktopCoordinate(y, g.DeclaredHeight, g.NativeHeight)
}

// NativePointToDeclared maps a point from native desktop coordinates to the
// declared model-facing coordinate space using the same truncating transform.
func (g DesktopGeometry) NativePointToDeclared(x, y int) (declaredX, declaredY int) {
	return scaleDesktopCoordinate(x, g.NativeWidth, g.DeclaredWidth),
		scaleDesktopCoordinate(y, g.NativeHeight, g.DeclaredHeight)
}

func computeDeclaredDesktopSize(nativeWidth, nativeHeight int) (declaredWidth, declaredHeight int) {
	if desktopSizeFitsDeclaredLimits(nativeWidth, nativeHeight) {
		return nativeWidth, nativeHeight
	}

	if nativeWidth >= nativeHeight {
		for _, declaredWidth := range preferredDeclaredDesktopWidths {
			if declaredWidth > nativeWidth {
				continue
			}

			declaredHeight := max(1, declaredWidth*nativeHeight/nativeWidth)
			if desktopSizeFitsDeclaredLimits(declaredWidth, declaredHeight) {
				return declaredWidth, declaredHeight
			}
		}
	}

	return computeGenericDeclaredDesktopSize(nativeWidth, nativeHeight)
}

func desktopSizeFitsDeclaredLimits(width, height int) bool {
	return max(width, height) <= desktopDeclaredMaxLongEdge &&
		width*height <= desktopDeclaredMaxTotalPixels
}

func computeGenericDeclaredDesktopSize(width, height int) (scaledWidth, scaledHeight int) {
	longEdge := max(width, height)
	totalPixels := width * height
	longEdgeScale := float64(desktopDeclaredMaxLongEdge) / float64(longEdge)
	totalPixelsScale := math.Sqrt(
		float64(desktopDeclaredMaxTotalPixels) / float64(totalPixels),
	)
	scale := min(1.0, longEdgeScale, totalPixelsScale)

	if scale >= 1.0 {
		return width, height
	}

	return max(1, int(float64(width)*scale)),
		max(1, int(float64(height)*scale))
}

func scaleDesktopCoordinate(coord, fromDim, toDim int) int {
	if toDim <= 0 {
		return 0
	}
	if fromDim <= 0 || fromDim == toDim {
		return clampDesktopCoordinate(coord, toDim)
	}

	scaled := (float64(coord)+0.5)*float64(toDim)/float64(fromDim) - 0.5
	scaled = math.Max(scaled, 0)
	scaled = math.Min(scaled, float64(toDim-1))
	return int(math.Round(scaled))
}

func clampDesktopCoordinate(coord, dim int) int {
	if dim <= 0 {
		return 0
	}
	if coord < 0 {
		return 0
	}
	if coord >= dim {
		return dim - 1
	}
	return coord
}

func sanitizeDesktopDimension(dim int) int {
	if dim <= 0 {
		return 1
	}
	return dim
}
