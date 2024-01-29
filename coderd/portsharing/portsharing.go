package portsharing

type PortSharer interface {
	CanRestrictShareLevel() bool
}

type AGPLPortSharer struct{}

func (AGPLPortSharer) CanRestrictShareLevel() bool {
	return false
}

var DefaultPortSharer PortSharer = AGPLPortSharer{}
