package codersdk

// ExperimentalClient is a client for the experimental API.
// Its interface is not guaranteed to be stable and may change at any time.
// @typescript-ignore ExperimentalClient
type ExperimentalClient struct {
	*Client
}

func NewExperimentalClient(client *Client) *ExperimentalClient {
	return &ExperimentalClient{
		Client: client,
	}
}
