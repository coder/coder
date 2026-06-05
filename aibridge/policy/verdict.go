package policy

// Verdict is the outcome of a decision policy. Verdicts compose across a
// pipeline by precedence: BLOCK > FLAG > LOG > ALLOW.
type Verdict string

const (
	VerdictAllow Verdict = "ALLOW"
	VerdictLog   Verdict = "LOG"
	VerdictFlag  Verdict = "FLAG"
	VerdictBlock Verdict = "BLOCK"
)

// rank orders verdicts for reduction. Unknown values rank as ALLOW.
func (v Verdict) rank() int {
	switch v {
	case VerdictBlock:
		return 3
	case VerdictFlag:
		return 2
	case VerdictLog:
		return 1
	default:
		return 0
	}
}

// Blocks reports whether the verdict stops the request.
func (v Verdict) Blocks() bool { return v == VerdictBlock }

// ReduceVerdicts combines verdicts by precedence BLOCK > FLAG > LOG > ALLOW.
// With no verdicts it returns ALLOW.
func ReduceVerdicts(vs ...Verdict) Verdict {
	out := VerdictAllow
	for _, v := range vs {
		if v.rank() > out.rank() {
			out = v
		}
	}
	return out
}
