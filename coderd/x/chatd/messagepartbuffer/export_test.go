package messagepartbuffer

// EpisodeCount returns the number of tracked episodes so tests can assert
// that episode state is reclaimed and does not leak.
func (b *Buffer) EpisodeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.episodes)
}
