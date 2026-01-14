//nolint:revive,gocritic,errname,unconvert
package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

const (
	CAKeyName  = "ca-key.pem"
	CACertName = "ca-cert.pem"
)

type UserInfo struct {
	SudoUser  string
	Uid       int
	Gid       int
	HomeDir   string
	ConfigDir string
}

// GetUserInfo returns information about the current user, handling sudo scenarios
func GetUserInfo() *UserInfo {
	// Only consider SUDO_USER if we're actually running with elevated privileges
	// In environments like Coder workspaces, SUDO_USER may be set to 'root'
	// but we're not actually running under sudo
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && os.Geteuid() == 0 && sudoUser != "root" {
		// We're actually running under sudo with a non-root original user
		user, err := user.Lookup(sudoUser)
		if err != nil {
			return getCurrentUserInfo() // Fallback to current user
		}

		uid, _ := strconv.Atoi(os.Getenv("SUDO_UID"))
		gid, _ := strconv.Atoi(os.Getenv("SUDO_GID"))

		// If we couldn't get UID/GID from env, parse from user info
		if uid == 0 {
			if parsedUID, err := strconv.Atoi(user.Uid); err == nil {
				uid = parsedUID
			}
		}
		if gid == 0 {
			if parsedGID, err := strconv.Atoi(user.Gid); err == nil {
				gid = parsedGID
			}
		}

		configDir := getConfigDir(user.HomeDir)

		return &UserInfo{
			SudoUser:  sudoUser,
			Uid:       uid,
			Gid:       gid,
			HomeDir:   user.HomeDir,
			ConfigDir: configDir,
		}
	}

	// Not actually running under sudo, use current user
	return getCurrentUserInfo()
}

// getCurrentUserInfo gets information for the current user
func getCurrentUserInfo() *UserInfo {
	currentUser, err := user.Current()
	if err != nil {
		// Fallback with empty values if we can't get user info
		return &UserInfo{}
	}

	uid, _ := strconv.Atoi(currentUser.Uid)
	gid, _ := strconv.Atoi(currentUser.Gid)

	configDir := getConfigDir(currentUser.HomeDir)

	return &UserInfo{
		SudoUser:  currentUser.Username,
		Uid:       uid,
		Gid:       gid,
		HomeDir:   currentUser.HomeDir,
		ConfigDir: configDir,
	}
}

// getConfigDir determines the config directory based on XDG_CONFIG_HOME or fallback
func getConfigDir(homeDir string) string {
	// Use XDG_CONFIG_HOME if set, otherwise fallback to ~/.config/coder_boundary
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "coder_boundary")
	}
	return filepath.Join(homeDir, ".config", "coder_boundary")
}

func (u *UserInfo) CAKeyPath() string {
	return filepath.Join(u.ConfigDir, CAKeyName)
}

func (u *UserInfo) CACertPath() string {
	return filepath.Join(u.ConfigDir, CACertName)
}
