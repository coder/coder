package srverr

// Error is an interface for specifying how specific errors should be
// dispatched by the API. The underlying struct is sent under the `details`
// field.
type Error interface {
	Status() int
	PublicMessage() string
	Code() Code
}
