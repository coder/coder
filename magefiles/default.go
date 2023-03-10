//go:build mage

package main

import (
	"os"

	"github.com/magefile/mage/mg"
)

func All() {
	mg.Deps((Fmt).All)
}

var Default = All

func inCI() bool {
	return os.Getenv("GITHUB_ENV") != "" || os.Getenv("CI") != ""
}
