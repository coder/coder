package codersdk

import "golang.org/x/xerrors"

type AppCORSBehavior string

const (
	AppCORSBehaviorSimple   AppCORSBehavior = "simple"
	AppCORSBehaviorPassthru AppCORSBehavior = "passthru"
)

func (c AppCORSBehavior) Validate() error {
	if c != AppCORSBehaviorSimple && c != AppCORSBehaviorPassthru {
		return xerrors.New("Invalid CORS behavior.")
	}
	return nil
}
