package coderdtest

import "github.com/coder/coder/v2/codersdk/wsjson"

// SynchronousStream returns a function that assumes the stream is synchronous.
// Meaning each request sent assumes exactly one response will be received.
// The function will block until the response is received or an error occurs.
//
// This should not be used in production code, as it does not handle edge cases.
// The second function `pop` can be used to retrieve the next response from the
// stream without sending a new request. This is useful for dynamic parameters
func SynchronousStream[R any, W any](stream *wsjson.Stream[R, W]) (do func(W) (R, error), pop func() R) {
	rec := stream.Chan()

	return func(req W) (R, error) {
			err := stream.Send(req)
			if err != nil {
				return *new(R), err
			}

			return <-rec, nil
		}, func() R {
			return <-rec
		}
}
