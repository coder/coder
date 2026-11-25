package proto

import "google.golang.org/grpc/codes"

// ErrCodeInternal is the dRPC error code (see drpcerr) that coderd sets on an
// IsAuthorized error caused by coderd itself, such as a failed database query,
// rather than by the request. A gateway answers it with a 500. An error
// without it is a rejection and is answered with a 403, which is also how a
// gateway treats every error from a coderd that does not set the code.
//
// The code travels with the error, not in a message, so it needs no change to
// the protocol version.
const ErrCodeInternal = uint64(codes.Internal)
