package filefinder

import "slices"

func toLowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func normalizeQuery(q string) string {
	b := make([]byte, 0, len(q))
	prevSpace := true
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
	if len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}
	return string(b)
}

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

func extractTrigrams(s []byte) []uint32 {
	if len(s) < 3 {
		return nil
	}
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
	slices.Sort(result)
	return result
}

func extractBasename(path []byte) (offset int, length int) {
	end := len(path)
	if end > 0 && path[end-1] == '/' {
		end--
	}
	if end == 0 {
		return 0, 0
	}
	i := end - 1
	for i >= 0 && path[i] != '/' {
		i--
	}
	start := i + 1
	return start, end - start
}

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

func prefix1(name []byte) byte {
	if len(name) == 0 {
		return 0
	}
	return toLowerASCII(name[0])
}

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
