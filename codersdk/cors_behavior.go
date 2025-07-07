package codersdk

import (
	"golang.org/x/xerrors"
)

type CORSBehavior string

const (
	CORSBehaviorSimple   CORSBehavior = "simple"
	CORSBehaviorPassthru CORSBehavior = "passthru"
)

func (c CORSBehavior) Validate() error {
	if c != CORSBehaviorSimple && c != CORSBehaviorPassthru {
		return xerrors.New("Invalid CORS behavior.")
	}
	return nil
}
