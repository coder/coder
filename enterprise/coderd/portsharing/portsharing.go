package portsharing

type EnterprisePortSharer struct {
}

func NewEnterprisePortSharer() *EnterprisePortSharer {
	return &EnterprisePortSharer{}
}

func (EnterprisePortSharer) CanRestrictSharing() bool {
	return true
}
