//go:build !windows

package cli

func installScriptFiles() []string {
	return []string{
		"install.sh",
		"install",
		"bootstrap.sh",
		"bootstrap",
		"setup.sh",
		"setup",
		"script/install.sh",
		"script/install",
		"script/bootstrap.sh",
		"script/bootstrap",
		"script/setup.sh",
		"script/setup",
	}
}
