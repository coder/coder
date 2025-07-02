package httperror

import (
	"errors"

	"github.com/coder/coder/v2/codersdk"
)

type CoderSDKError interface {
	Response() (int, codersdk.Response)
}

func IsCoderSDKError(err error) (CoderSDKError, bool) {
	var responseErr CoderSDKError
	if errors.As(err, &responseErr) {
		return responseErr, true
	}
	return nil, false
}
