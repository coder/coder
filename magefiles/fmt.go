//go:build mage

package main

import (
	"os/exec"
	"regexp"

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
	info, err := find(regexp.MustCompile(`\.sh$`))
	if err != nil {
		return err
	}
	flag := "-w"
	if inCI() {
		flag = "-d"
	}
	(&cmd{
		exec.Command(
			"shfmt",
			append([]string{flag}, info...)...,
		),
	}).run()
	return nil
}

func (Fmt) All() error {
	mg.Deps(Fmt.Go, Fmt.Terraform, Fmt.Prettier, Fmt.Sh)
	return nil
}
