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

func (Fmt) Sh() error {
	names, err := find(`\.sh$`)
	if err != nil {
		return err
	}

	flag := "-w"
	if inCI() {
		flag = "-d"
	}
	return goRun(
		"mvdan.cc/sh/v3/cmd/shfmt@v3.5.0",
		append([]string{flag}, names...)...,
	).run()
}

func (Fmt) All() error {
	mg.Deps(Fmt.Go, Fmt.Terraform, Fmt.Prettier, Fmt.Sh)
	return nil
}
