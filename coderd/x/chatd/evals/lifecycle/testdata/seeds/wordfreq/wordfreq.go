// Package wordfreq counts how often words appear in text.
package wordfreq

import "strings"

// Count returns the number of times each word appears in text. Words are
// compared case-insensitively and surrounding punctuation is ignored.
func Count(text string) map[string]int {
	counts := map[string]int{}
	for _, field := range strings.Fields(text) {
		word := strings.Trim(field, ".,;:!?\"'()")
		if word == "" {
			continue
		}
		counts[word]++
	}
	return counts
}

// Top returns the n most frequent words, most frequent first. Ties are
// broken alphabetically.
func Top(counts map[string]int, n int) []string {
	words := make([]string, 0, len(counts))
	for w := range counts {
		words = append(words, w)
	}
	sortWords(words, counts)
	if len(words) > n {
		words = words[:n]
	}
	return words
}

func sortWords(words []string, counts map[string]int) {
	for i := 1; i < len(words); i++ {
		for j := i; j > 0 && less(words[j], words[j-1], counts); j-- {
			words[j], words[j-1] = words[j-1], words[j]
		}
	}
}

func less(a, b string, counts map[string]int) bool {
	if counts[a] != counts[b] {
		return counts[a] > counts[b]
	}
	return a < b
}
