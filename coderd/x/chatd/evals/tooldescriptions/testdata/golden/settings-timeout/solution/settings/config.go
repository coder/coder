// Package settings holds the sync service defaults.
package settings

// Config controls how the sync service talks to its endpoint.
type Config struct {
	// Timeout is the request timeout in seconds.
	Timeout    int
	RetryLimit int
}

// Default is the profile for the primary endpoint.
func Default() Config {
	return Config{
		Timeout:    45,
		RetryLimit: 3,
	}
}

// Fallback is the profile for the fallback endpoint.
func Fallback() Config {
	return Config{
		Timeout:    30,
		RetryLimit: 3,
	}
}
