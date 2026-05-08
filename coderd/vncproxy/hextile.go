package vncproxy

import (
	"io"

	"golang.org/x/xerrors"
)

// Hextile sub-encoding flag bits per RFC 6143 §7.7.4.
const (
	hextileRaw                 byte = 0x01
	hextileBackgroundSpecified byte = 0x02
	hextileForegroundSpecified byte = 0x04
	hextileAnySubrects         byte = 0x08
	hextileSubrectsColoured    byte = 0x10
)

// streamHextile parses and forwards a Hextile-encoded rectangle of
// width x height pixels. Hextile is a per-tile encoding: the rectangle
// is partitioned into 16x16 tiles (the right and bottom edges may be
// smaller). Each tile begins with a 1-byte sub-encoding mask whose
// flags determine the rest of the tile's body length.
//
// Caller has already forwarded the 12-byte rectangle header. This
// function reads the rest of the rectangle from src and writes it to
// dst, then returns. It does not buffer the whole rectangle.
func streamHextile(width, height, bytesPerPixel int, src io.Reader, dst io.Writer) error {
	for ty := 0; ty < height; ty += 16 {
		th := 16
		if ty+th > height {
			th = height - ty
		}
		for tx := 0; tx < width; tx += 16 {
			tw := 16
			if tx+tw > width {
				tw = width - tx
			}
			if err := streamHextileTile(tw, th, bytesPerPixel, src, dst); err != nil {
				return xerrors.Errorf("tile (%d,%d): %w", tx, ty, err)
			}
		}
	}
	return nil
}

// streamHextileTile forwards exactly one Hextile tile of tileWidth x
// tileHeight pixels. The on-the-wire layout is:
//
//	1 byte sub-encoding mask
//	if Raw set: tileWidth * tileHeight * bytesPerPixel pixel bytes.
//	  No other fields are present in this case.
//	if BackgroundSpecified set: bytesPerPixel background pixel.
//	if ForegroundSpecified set: bytesPerPixel foreground pixel.
//	if AnySubrects set:
//	  1 byte number-of-subrectangles
//	  per subrect:
//	    if SubrectsColoured set: bytesPerPixel + 2 bytes (x,y,w,h packed)
//	    else: 2 bytes (x,y,w,h packed)
//
// We do not interpret the pixels, just route them.
func streamHextileTile(tileWidth, tileHeight, bytesPerPixel int, src io.Reader, dst io.Writer) error {
	maskBuf, err := readExact(src, 1)
	if err != nil {
		return xerrors.Errorf("read sub-encoding mask: %w", err)
	}
	if err := writeAll(dst, maskBuf); err != nil {
		return xerrors.Errorf("forward sub-encoding mask: %w", err)
	}
	mask := maskBuf[0]

	if mask&hextileRaw != 0 {
		// All other flag bits are ignored when Raw is set.
		body := tileWidth * tileHeight * bytesPerPixel
		return forwardExact(src, dst, body)
	}

	if mask&hextileBackgroundSpecified != 0 {
		if err := forwardExact(src, dst, bytesPerPixel); err != nil {
			return xerrors.Errorf("forward background pixel: %w", err)
		}
	}
	if mask&hextileForegroundSpecified != 0 {
		if err := forwardExact(src, dst, bytesPerPixel); err != nil {
			return xerrors.Errorf("forward foreground pixel: %w", err)
		}
	}
	if mask&hextileAnySubrects == 0 {
		return nil
	}

	countBuf, err := readExact(src, 1)
	if err != nil {
		return xerrors.Errorf("read subrect count: %w", err)
	}
	if err := writeAll(dst, countBuf); err != nil {
		return xerrors.Errorf("forward subrect count: %w", err)
	}
	count := int(countBuf[0])
	subSize := 2
	if mask&hextileSubrectsColoured != 0 {
		subSize = bytesPerPixel + 2
	}
	body := count * subSize
	if body == 0 {
		return nil
	}
	return forwardExact(src, dst, body)
}
