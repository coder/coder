package intercept

// OpenAI error type and code constants used by the chatcompletions
// and responses interceptors. The OpenAI Go SDK does not expose
// these as typed constants, so we define our own.
// See https://platform.openai.com/docs/guides/error-codes.
const (
	OpenAIErrTypeError     = "error"
	OpenAIErrTypeAPI       = "api_error"
	OpenAIErrTypeRateLimit = "rate_limit_error"

	OpenAIErrCodeServer    = "server_error"
	OpenAIErrCodeRateLimit = "rate_limit_exceeded"
)
