package filefinder

import (
	"hash/fnv"
	"sort"
)

// toLowerASCII returns the lowercase form of an ASCII letter, or
// the byte unchanged if it is not an uppercase ASCII letter.
func toLowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// normalizeQuery trims spaces, lowercases ASCII bytes, collapses
// multiple spaces into one, and converts backslashes to forward
// slashes. Non-ASCII bytes are passed through unchanged.
func normalizeQuery(q string) string {
	b := make([]byte, 0, len(q))
	prevSpace := true // start true to trim leading spaces
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == '\\' {
			c = '/'
		}
		c = toLowerASCII(c)
		if c == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b = append(b, c)
	}
	// Trim trailing space.
	if len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}
	return string(b)
}

// normalizePathBytes lowercases ASCII bytes, converts backslashes
// to forward slashes, and collapses multiple slashes. The
// operation is performed in-place on the provided slice and the
// resulting (possibly shorter) slice is returned.
func normalizePathBytes(p []byte) []byte {
	j := 0
	prevSlash := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '\\' {
			c = '/'
		}
		c = toLowerASCII(c)
		if c == '/' {
			if prevSlash {
				continue
			}
			prevSlash = true
		} else {
			prevSlash = false
		}
		p[j] = c
		j++
	}
	return p[:j]
}

// extractTrigrams extracts unique trigrams from a byte slice. Each
// byte is lowercased for case-insensitive matching before the
// trigram is packed as (b0<<16 | b1<<8 | b2). The returned slice
// is sorted and deduplicated.
func extractTrigrams(s []byte) []uint32 {
	if len(s) < 3 {
		return nil
	}

	// Use a map for deduplication. For very long strings a bitset
	// on the 24-bit space would be faster, but a map is simpler
	// and correct for all inputs.
	seen := make(map[uint32]struct{}, len(s))
	for i := 0; i <= len(s)-3; i++ {
		b0 := toLowerASCII(s[i])
		b1 := toLowerASCII(s[i+1])
		b2 := toLowerASCII(s[i+2])
		gram := uint32(b0)<<16 | uint32(b1)<<8 | uint32(b2)
		seen[gram] = struct{}{}
	}

	result := make([]uint32, 0, len(seen))
	for g := range seen {
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

// extractBasename returns the offset and length of the basename
// component within path. The basename is the portion after the
// last '/' separator. If the path ends with '/', the trailing
// slash is ignored.
func extractBasename(path []byte) (offset int, length int) {
	end := len(path)
	// Skip trailing slash.
	if end > 0 && path[end-1] == '/' {
		end--
	}
	if end == 0 {
		return 0, 0
	}

	// Scan backwards for the last separator.
	i := end - 1
	for i >= 0 && path[i] != '/' {
		i--
	}
	// i is now either -1 (no slash) or the index of the last
	// slash before the basename.
	start := i + 1
	return start, end - start
}

// extractSegments splits a path by '/' and returns all non-empty
// segments.
func extractSegments(path []byte) [][]byte {
	var segments [][]byte
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				segments = append(segments, path[start:i])
			}
			start = i + 1
		}
	}
	return segments
}

// hashBasename computes an FNV-64a hash of the provided basename
// bytes.
func hashBasename(name []byte) uint64 {
	h := fnv.New64a()
	// fnv hash.Write never returns an error.
	_, _ = h.Write(name)
	return h.Sum64()
}

// prefix1 returns the first byte of the basename, lowered. Returns
// 0 if name is empty.
func prefix1(name []byte) byte {
	if len(name) == 0 {
		return 0
	}
	return toLowerASCII(name[0])
}

// prefix2 returns the first two bytes of the basename packed into
// a uint16 (first byte in high 8 bits, second in low 8 bits),
// both lowered. If name has only one byte, the low 8 bits are
// zero.
func prefix2(name []byte) uint16 {
	if len(name) == 0 {
		return 0
	}
	hi := uint16(toLowerASCII(name[0])) << 8
	if len(name) < 2 {
		return hi
	}
	return hi | uint16(toLowerASCII(name[1]))
}
