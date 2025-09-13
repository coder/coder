package codersdk

import (
	"io"
	"net/http"
	"net/url"

	"cdr.dev/slog"
)

// ClientBuilder is a builder for a Client.
// @typescript-ignore ClientBuilder
type ClientBuilder struct {
	serverURL                *url.URL
	httpClient               *http.Client
	sessionTokenProvider     SessionTokenProvider
	logger                   slog.Logger
	logBodies                bool
	plainLogger              io.Writer
	trace                    bool
	disableDirectConnections bool

	// unbuiltClient is the client that is built when Build is called. Used for cases where the code building the client
	// needs to reference the client while building it.
	unbuiltClient *Client
}

func NewClientBuilder(serverURL *url.URL) *ClientBuilder {
	return &ClientBuilder{
		serverURL:            serverURL,
		httpClient:           &http.Client{},
		sessionTokenProvider: FixedSessionTokenProvider{},
		unbuiltClient:        &Client{},
	}
}

func (b *ClientBuilder) SessionToken(sessionToken string) *ClientBuilder {
	b.sessionTokenProvider = FixedSessionTokenProvider{SessionToken: sessionToken}
	return b
}

func (b *ClientBuilder) SessionTokenProvider(sessionTokenProvider SessionTokenProvider) *ClientBuilder {
	b.sessionTokenProvider = sessionTokenProvider
	return b
}

func (b *ClientBuilder) Logger(logger slog.Logger) *ClientBuilder {
	b.logger = logger
	return b
}

func (b *ClientBuilder) LogBodies(logBodies bool) *ClientBuilder {
	b.logBodies = logBodies
	return b
}

func (b *ClientBuilder) PlainLogger(plainLogger io.Writer) *ClientBuilder {
	b.plainLogger = plainLogger
	return b
}

func (b *ClientBuilder) Trace() *ClientBuilder {
	b.trace = true
	return b
}

func (b *ClientBuilder) DisableDirectConnections() *ClientBuilder {
	b.disableDirectConnections = true
	return b
}

func (b *ClientBuilder) HTTPClient(httpClient *http.Client) *ClientBuilder {
	b.httpClient = httpClient
	return b
}

// DangerouslyGetUnbuiltClient returns the unbuilt client.
//
// This is dangerous to use because the client can be modified after it is built. Only use this if you need a reference
// to the client during the building process, e.g. to give to a client component like the SessionTokenProvider.
func (b *ClientBuilder) DangerouslyGetUnbuiltClient() *Client {
	return b.unbuiltClient
}

func (b *ClientBuilder) Build() *Client {
	client := b.unbuiltClient
	b.unbuiltClient = &Client{} // clear this so that the client cannot be modified after it is built
	client.URL = b.serverURL
	client.HTTPClient = b.httpClient
	client.SessionTokenProvider = b.sessionTokenProvider
	client.logger = b.logger
	client.logBodies = b.logBodies
	client.PlainLogger = b.plainLogger
	client.Trace = b.trace
	client.DisableDirectConnections = b.disableDirectConnections
	return client
}
