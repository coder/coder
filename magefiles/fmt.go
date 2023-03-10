//go:build mage

package main

import (
	"github.com/magefile/mage/mg"
)

type Fmt mg.Namespace

func (Fmt) Go() error {
	return shell(
		"go run mvdan.cc/gofumpt@v0.4.0 -w -l .",
	).run()
}

func (Fmt) Terraform() error {
	return shell(
		"terraform fmt -recursive",
	).run()
}

func (Fmt) Prettier() error {
	var cmd *cmd
	if inCI() {
		cmd = shell("yarn run format:check")
	} else {
		cmd = shell("yarn run format:write")
	}
	return cmd.cd("site").run()
}

func (Fmt) All() error {
	mg.Deps(Fmt.Go, Fmt.Terraform, Fmt.Prettier)
	return nil
}
