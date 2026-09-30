package responsesws

// StateSizes returns the number of entries in each of the session's
// bookkeeping maps that holds any, keyed by map name.
func StateSizes(s *Session) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sizes := map[string]int{
		"open":          len(s.open),
		"pending":       len(s.pending),
		"continuations": len(s.continuations),
		"active":        len(s.active),
		"responses":     len(s.responses),
		"steered":       len(s.steered),
	}
	for name, n := range sizes {
		if n == 0 {
			delete(sizes, name)
		}
	}
	return sizes
}
