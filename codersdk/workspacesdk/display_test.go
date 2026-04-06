package workspacesdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestNewDesktopGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		nativeWidth    int
		nativeHeight   int
		declaredWidth  int
		declaredHeight int
	}{
		{
			name:           "1366x768_keeps_native_geometry",
			nativeWidth:    1366,
			nativeHeight:   768,
			declaredWidth:  1366,
			declaredHeight: 768,
		},
		{
			name:           "1920x1080_prefers_1280x720",
			nativeWidth:    1920,
			nativeHeight:   1080,
			declaredWidth:  1280,
			declaredHeight: 720,
		},
		{
			name:           "1920x1200_prefers_1280x800",
			nativeWidth:    1920,
			nativeHeight:   1200,
			declaredWidth:  1280,
			declaredHeight: 800,
		},
		{
			name:           "2048x1536_prefers_1024x768",
			nativeWidth:    2048,
			nativeHeight:   1536,
			declaredWidth:  1024,
			declaredHeight: 768,
		},
		{
			name:           "3840x2160_prefers_1280x720",
			nativeWidth:    3840,
			nativeHeight:   2160,
			declaredWidth:  1280,
			declaredHeight: 720,
		},
		{
			name:           "1568x1000_prefers_1280x816",
			nativeWidth:    1568,
			nativeHeight:   1000,
			declaredWidth:  1280,
			declaredHeight: 816,
		},
		{
			name:           "portrait_falls_back_to_generic_scaling",
			nativeWidth:    1000,
			nativeHeight:   2000,
			declaredWidth:  758,
			declaredHeight: 1516,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			geometry := workspacesdk.NewDesktopGeometry(
				tt.nativeWidth,
				tt.nativeHeight,
			)

			assert.Equal(t, tt.nativeWidth, geometry.NativeWidth)
			assert.Equal(t, tt.nativeHeight, geometry.NativeHeight)
			assert.Equal(t, tt.declaredWidth, geometry.DeclaredWidth)
			assert.Equal(t, tt.declaredHeight, geometry.DeclaredHeight)
			assert.LessOrEqual(t, max(geometry.DeclaredWidth, geometry.DeclaredHeight), 1568)
			assert.LessOrEqual(t, geometry.DeclaredWidth*geometry.DeclaredHeight, 1_150_000)
		})
	}
}

func TestDefaultDesktopGeometry(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()

	assert.Equal(t, workspacesdk.DesktopNativeWidth, geometry.NativeWidth)
	assert.Equal(t, workspacesdk.DesktopNativeHeight, geometry.NativeHeight)
	assert.Equal(t, 1280, geometry.DeclaredWidth)
	assert.Equal(t, 720, geometry.DeclaredHeight)
}

func TestDesktopGeometryDeclaredPointToNative(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.NewDesktopGeometryWithDeclared(1920, 1080, 1280, 720)

	tests := []struct {
		name  string
		x     int
		y     int
		wantX int
		wantY int
	}{
		{
			name:  "origin",
			x:     0,
			y:     0,
			wantX: 0,
			wantY: 0,
		},
		{
			name:  "center",
			x:     640,
			y:     360,
			wantX: 960,
			wantY: 540,
		},
		{
			name:  "max_coordinate_maps_to_last_native_pixel",
			x:     1279,
			y:     719,
			wantX: 1919,
			wantY: 1079,
		},
		{
			name:  "out_of_bounds_values_are_clamped",
			x:     5000,
			y:     -5,
			wantX: 1919,
			wantY: 0,
		},
		{
			name:  "rounding_applies",
			x:     853,
			y:     402,
			wantX: 1280,
			wantY: 603,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotX, gotY := geometry.DeclaredPointToNative(tt.x, tt.y)
			assert.Equal(t, tt.wantX, gotX)
			assert.Equal(t, tt.wantY, gotY)
		})
	}
}

func TestDesktopGeometryNativePointToDeclared(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.NewDesktopGeometryWithDeclared(1920, 1080, 1366, 768)

	tests := []struct {
		name  string
		x     int
		y     int
		wantX int
		wantY int
	}{
		{
			name:  "origin",
			x:     0,
			y:     0,
			wantX: 0,
			wantY: 0,
		},
		{
			name:  "center",
			x:     960,
			y:     540,
			wantX: 683,
			wantY: 384,
		},
		{
			name:  "bottom_right_maps_to_last_pixel",
			x:     1919,
			y:     1079,
			wantX: 1365,
			wantY: 767,
		},
		{
			name:  "out_of_bounds_values_are_clamped",
			x:     -10,
			y:     5000,
			wantX: 0,
			wantY: 767,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotX, gotY := geometry.NativePointToDeclared(tt.x, tt.y)
			assert.Equal(t, tt.wantX, gotX)
			assert.Equal(t, tt.wantY, gotY)
		})
	}
}
