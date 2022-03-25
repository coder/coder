package config

import (
	"fmt"
	"os"
	"strconv"
)

// Specify Key constants for EnvConfig here.
const (
	EnvCoderAccessURL          Key = "CODER_ACCESS_URL"
	EnvCoderAddress            Key = "CODER_ADDRESS"
	EnvCoderDevMode            Key = "CODER_DEV_MODE"
	EnvCoderPGConnectionURL    Key = "CODER_PG_CONNECTION_URL"
	EnvCoderProvisionerDaemons Key = "CODER_PROVISIONER_DAEMONS"
	EnvCoderTLSEnable          Key = "CODER_TLS_ENABLE"
	EnvCoderTLSCertFile        Key = "CODER_TLS_CERT_FILE"
	EnvCoderTLSClientCAFile    Key = "CODER_TLS_CLIENT_CA_FILE"
	EnvCoderTLSClientAuth      Key = "CODER_TLS_CLIENT_AUTH"
	EnvCoderTLSKeyFile         Key = "CODER_TLS_KEY_FILE"
	EnvCoderTLSMinVersion      Key = "CODER_TLS_MIN_VERSION"
	EnvCoderDevTunnel          Key = "CODER_DEV_TUNNEL"
)

// EnvConfigSchema is the global schema for environment variables.
// This schema is used when config.ReadEnvConfig() is called.
// Add new items to the array and fill in Key, DefaultValue, and Usage fields.
//
// Example:
// var (
//	  root *cobra.Command
//    accessURL string
//    ec = config.ReadEnvironmentConfig()
// )
//
// root.Flags().StringVarP(&accessURL, "access-url", "", ec.GetString(config.EnvCoderAccessURL), ec.Usage(config.EnvCoderAccessURL))
//
var EnvConfigSchema = EnvConfigSchemaFields{
	{
		Key:          EnvCoderAccessURL,
		DefaultValue: "",
		Usage:        "Specifies the external URL to access Coder",
	},
	{
		Key:          EnvCoderAddress,
		DefaultValue: "127.0.0.1:3000",
		Usage:        "The address to serve the API and dashboard",
	},
	{
		Key:          EnvCoderDevMode,
		DefaultValue: "",
		Usage:        "Serve Coder in dev mode for tinkering",
	},
	{
		Key:          EnvCoderPGConnectionURL,
		DefaultValue: "",
		Usage:        "URL of a PostgreSQL database to connect to",
	},
	{
		Key:          EnvCoderProvisionerDaemons,
		DefaultValue: "1",
		Usage:        "The amount of provisioner daemons to create on start",
	},
	{
		Key:          EnvCoderTLSEnable,
		DefaultValue: "",
		Usage:        "Specifies if TLS will be enabled",
	},
	{
		Key:          EnvCoderTLSCertFile,
		DefaultValue: "",
		Usage: "Specifies the path to the certificate for TLS. It requires a PEM-encoded file. " +
			"To configure the listener to use a CA certificate, concatenate the primary certificate " +
			"and the CA certificate together. The primary certificate should appear first in the combined file",
	},
	{
		Key:          EnvCoderTLSClientCAFile,
		DefaultValue: "",
		Usage:        "PEM-encoded Certificate Authority file used for checking the authenticity of client",
	},
	{
		Key:          EnvCoderTLSClientAuth,
		DefaultValue: "request",
		Usage: `Specifies the policy the server will follow for TLS Client Authentication. ` +
			`Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify"`,
	},
	{
		Key:          EnvCoderTLSKeyFile,
		DefaultValue: "",
		Usage:        "Specifies the path to the private key for the certificate. It requires a PEM-encoded file",
	},
	{
		Key:          EnvCoderTLSMinVersion,
		DefaultValue: "tls12",
		Usage:        `Specifies the minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"`,
	},
	{
		Key:          EnvCoderDevTunnel,
		DefaultValue: "true",
		Usage:        "Serve dev mode through a Cloudflare Tunnel for easy setup ",
	},
}

// ReadEnvConfig reads the application environment variables and stores them in memory.
func ReadEnvConfig() EnvConfig {
	return EnvConfigSchema.readEnvironment()
}

// Key is the environment variable key.
type Key string

// EnvConfig allows reading of environment configuration.
type EnvConfig map[Key]EnvConfigSchemaField

// EnvConfigSchemaFields is the configuration schema for an EnvConfig.
type EnvConfigSchemaFields []EnvConfigSchemaField

// EnvConfigSchemaField represents an environment variable.
type EnvConfigSchemaField struct {
	Key          Key
	DefaultValue string
	Usage        string

	value string
}

func (e EnvConfigSchemaFields) readEnvironment() EnvConfig {
	var c = make(map[Key]EnvConfigSchemaField)
	for _, f := range e {
		s, ok := os.LookupEnv(string(f.Key))
		if !ok {
			s = f.DefaultValue
		}

		f.value = s
		c[f.Key] = f
	}

	return c
}

// GetString looks up the Key and returns the value as a string.
func (e EnvConfig) GetString(key Key) string {
	return e[key].value
}

// GetBool looks up the Key and returns the value as a bool.
func (e EnvConfig) GetBool(key Key) bool {
	b, _ := strconv.ParseBool(e[key].value)
	return b
}

// GetInt looks up the Key and returns the value as a int.
func (e EnvConfig) GetInt(key Key) int {
	b, _ := strconv.ParseInt(e[key].value, 10, 0)
	return int(b)
}

// Usage looks up the Key and returns the formatted usage message.
func (e EnvConfig) Usage(key Key) string {
	return fmt.Sprintf("%s (uses $%s).", e[key].Usage, key)
}
