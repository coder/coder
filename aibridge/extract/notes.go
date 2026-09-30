package extract

import "fmt"

// maxParseNotes bounds the notes kept per response so a stream of malformed
// events cannot grow memory without limit.
const maxParseNotes = 16

// ParseNotes collects parse notes, keeping the first few and counting the
// rest. The zero value is ready to use. Notes must not quote traffic
// content: they are logged.
type ParseNotes struct {
	notes   []string
	dropped int
}

// Addf records a note.
func (n *ParseNotes) Addf(format string, args ...any) {
	if len(n.notes) >= maxParseNotes {
		n.dropped++
		return
	}
	n.notes = append(n.notes, fmt.Sprintf(format, args...))
}

// List returns the recorded notes, with a summary of dropped notes last.
func (n *ParseNotes) List() []string {
	if len(n.notes) == 0 {
		return nil
	}
	out := make([]string, len(n.notes), len(n.notes)+1)
	copy(out, n.notes)
	if n.dropped > 0 {
		out = append(out, fmt.Sprintf("%d more parse notes dropped", n.dropped))
	}
	return out
}
