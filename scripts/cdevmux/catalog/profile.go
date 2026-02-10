package catalog

// Profile defines a preset configuration of services to run.
type Profile struct {
	Name        string
	Description string
	Services    []Service
}

// DefaultProfile returns the minimal development setup.
func DefaultProfile() *Profile {
	return &Profile{
		Name:        "default",
		Description: "Minimal setup: build-slim + database + coderd",
		Services: []Service{
			NewBuildSlim(),
			NewDatabase(),
			NewCoderd(),
		},
	}
}

// FullProfile returns a full-featured development setup.
func FullProfile() *Profile {
	return &Profile{
		Name:        "full",
		Description: "Full setup: build-slim + database + coderd + wsproxy + oidc + external provisioners",
		Services: []Service{
			NewBuildSlim(),
			NewDatabase(),
			NewCoderd(),
			NewWSProxy(),
			NewOIDC(OIDCVariantFake),
			NewProvisioner(),
		},
	}
}

// Profiles returns all available profiles.
func Profiles() map[string]*Profile {
	return map[string]*Profile{
		"default": DefaultProfile(),
		"full":    FullProfile(),
	}
}
