package portsharing

type PortSharer interface {
	Enabled() bool
}

type AGPLPortSharer struct{}

func (AGPLPortSharer) Enabled() bool {
	return true
}

var DefaultPortSharer PortSharer = AGPLPortSharer{}
