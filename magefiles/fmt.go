//go:build mage

package main

import (
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Fmt mg.Namespace

func (Fmt) Go() error {
	return sh.Run(
		"go", "run", "mvdan.cc/gofumpt@v0.4.0", "-w", "-l", ".",
	)
}

func (Fmt) Terraform() error {
	return sh.Run(
		"terraform", "fmt", "-recursive",
	)
}

func (Fmt) All() error {
	mg.Deps(Fmt.Go, Fmt.Terraform)
	return nil
}
