package codersdk

import (
	"context"
	"encoding/json"
)

type sdkRequestArgs struct {
	Method     string
	URL        string
	Body       any
	ReqOpts    []RequestOption
	ExpectCode int
}

type noResponse struct{}

func makeSDKRequest[T any](ctx context.Context, cli *Client, req sdkRequestArgs) (T, error) {
	var empty T
	res, err := cli.Request(ctx, req.Method, req.URL, req.Body, req.ReqOpts...)
	if err != nil {
		return empty, err
	}
	defer res.Body.Close()

	if res.StatusCode != req.ExpectCode {
		return empty, ReadBodyAsError(res)
	}

	switch (any)(empty).(type) {
	case noResponse:
		// noResponse means the caller does not care about the response body
		return empty, nil
	default:
	}
	var result T
	return result, json.NewDecoder(res.Body).Decode(&result)
}
