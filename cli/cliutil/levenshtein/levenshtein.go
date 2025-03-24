package levenshtein

import (
	"golang.org/x/exp/constraints"
	"golang.org/x/xerrors"
)

// Matches returns the closest matches to the needle from the haystack.
// The maxDistance parameter is the maximum Matches distance to consider.
// If no matches are found, an empty slice is returned.
func Matches(needle string, maxDistance int, haystack ...string) (matches []string) {
	for _, hay := range haystack {
		if d, err := Distance(needle, hay, maxDistance); err == nil && d <= maxDistance {
			matches = append(matches, hay)
		}
	}

	return matches
}

var ErrMaxDist = xerrors.New("levenshtein: maxDist exceeded")

// Distance returns the edit distance between a and b using the
// Wagner-Fischer algorithm.
// A and B must be less than 255 characters long.
// maxDist is the maximum distance to consider.
// A value of -1 for maxDist means no maximum.
func Distance(a, b string, maxDist int) (int, error) {
	if len(a) > 255 {
		return 0, xerrors.Errorf("levenshtein: a must be less than 255 characters long")
	}
	if len(b) > 255 {
		return 0, xerrors.Errorf("levenshtein: b must be less than 255 characters long")
	}
	// We've already checked that len(a) and len(b) are <= 255, so conversion is safe
	m := uint8(len(a)) // #nosec G115 -- length is checked to be <= 255
	n := uint8(len(b)) // #nosec G115 -- length is checked to be <= 255

	// Special cases for empty strings
	if m == 0 {
		return int(n), nil
	}
	if n == 0 {
		return int(m), nil
	}

	// Allocate a matrix of size m+1 * n+1
	d := make([][]uint8, 0)
	var i, j uint8
	for i = 0; i < m+1; i++ {
		di := make([]uint8, n+1)
		d = append(d, di)
	}

	// Source prefixes
	for i = 1; i < m+1; i++ {
		d[i][0] = i
	}

	// Target prefixes
	for j = 1; j < n; j++ {
		d[0][j] = j // nolint:gosec // this cannot overflow
	}

	// Compute the distance
	for j = 0; j < n; j++ {
		for i = 0; i < m; i++ {
			var subCost uint8
			// Equal
			if a[i] != b[j] {
				subCost = 1
			}
			// Don't forget: matrix is +1 size
			d[i+1][j+1] = min(
				d[i][j+1]+1,     // deletion
				d[i+1][j]+1,     // insertion
				d[i][j]+subCost, // substitution
			)
			// check maxDist on the diagonal
			if maxDist > -1 && i == j && maxDist <= 255 && d[i+1][j+1] > uint8(maxDist) { // #nosec G115 -- we check maxDist <= 255
				return int(d[i+1][j+1]), ErrMaxDist
			}
		}
	}

	return int(d[m][n]), nil
}

func min[T constraints.Ordered](ts ...T) T {
	if len(ts) == 0 {
		panic("min: no arguments")
	}
	m := ts[0]
	for _, t := range ts[1:] {
		if t < m {
			m = t
		}
	}
	return m
}
