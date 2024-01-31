package portsharing

type PortSharer interface {
	CanRestrictSharing() bool
}

type AGPLPortSharer struct{}

func (AGPLPortSharer) CanRestrictSharing() bool {
	return false
}

var DefaultPortSharer PortSharer = AGPLPortSharer{}
