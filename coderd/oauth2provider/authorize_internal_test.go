package oauth2provider

// Rejection reasons, exported for the oauth2provider_test package.
var (
	ReasonBeyondSession    = errBeyondSession.Error()
	ReasonSessionAllowList = errSessionAllowList.Error()
)
