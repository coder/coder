//go:build darwin

package sessionstore

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	// fixedUsername is the fixed username used for all keychain entries.
	// Since our interface only uses service names, we use a constant username.
	fixedUsername = "coder-login-credentials"

	execPathKeychain = "/usr/bin/security"
	notFoundStr      = "could not be found"
)

// operatingSystemKeyring implements keyringProvider for macOS.
// It is largely adapted from the zalando/go-keyring package.
type operatingSystemKeyring struct{}

func (operatingSystemKeyring) Set(service, credential string) error {
	// if the added secret has multiple lines or some non ascii,
	// macOS will hex encode it on return. To avoid getting garbage, we
	// encode all passwords
	password := base64.StdEncoding.EncodeToString([]byte(credential))

	cmd := exec.Command(execPathKeychain, "-i")
	stdIn, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err = cmd.Start(); err != nil {
		return err
	}

	command := fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n",
		shellEscape(service),
		shellEscape(fixedUsername),
		shellEscape(password))
	if len(command) > 4096 {
		return ErrSetDataTooBig
	}

	if _, err := io.WriteString(stdIn, command); err != nil {
		return err
	}

	if err = stdIn.Close(); err != nil {
		return err
	}

	return cmd.Wait()
}

func (operatingSystemKeyring) Get(service string) ([]byte, error) {
	out, err := exec.Command(
		execPathKeychain,
		"find-generic-password",
		"-s", service,
		"-wa", fixedUsername).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), notFoundStr) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	trimStr := strings.TrimSpace(string(out))
	return base64.StdEncoding.DecodeString(trimStr)
}

func (operatingSystemKeyring) Delete(service string) error {
	out, err := exec.Command(
		execPathKeychain,
		"delete-generic-password",
		"-s", service,
		"-a", fixedUsername).CombinedOutput()
	if strings.Contains(string(out), notFoundStr) {
		return os.ErrNotExist
	}
	return err
}

// shellEscape returns a shell-escaped version of the string s.
// This is adapted from github.com/zalando/go-keyring/internal/shellescape.
func shellEscape(s string) string {
	if len(s) == 0 {
		return "''"
	}

	pattern := regexp.MustCompile(`[^\w@%+=:,./-]`)
	if pattern.MatchString(s) {
		return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
	}

	return s
}
