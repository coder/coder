package codersdk

import "golang.org/x/xerrors"

type CORSBehavior string

const (
	AppCORSBehaviorSimple   CORSBehavior = "simple"
	AppCORSBehaviorPassthru CORSBehavior = "passthru"
)

func (c CORSBehavior) Validate() error {
	if c != AppCORSBehaviorSimple && c != AppCORSBehaviorPassthru {
		return xerrors.New("Invalid CORS behavior.")
	}
	return nil
}
