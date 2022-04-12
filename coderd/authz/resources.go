package authz

const WildcardSymbol = "*"

var (
	ResourceWorkspace = Object{
		Type: "workspace",
	}

	ResourceTemplate = Object{
		Type: "template",
	}

	// ResourceWildcard represents all resource types
	ResourceWildcard = Object{
		Type: WildcardSymbol,
	}
)
