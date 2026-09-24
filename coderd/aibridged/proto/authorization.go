package proto

// Authorization error codes are carried by dRPC errors so callers can
// distinguish denials from evaluation failures without inspecting messages.
const (
	AuthorizationErrorAuthentication uint64 = 1001
	AuthorizationErrorPolicy         uint64 = 1002
	AuthorizationErrorEvaluation     uint64 = 1003
	AuthorizationErrorMalformed      uint64 = 1004
)
