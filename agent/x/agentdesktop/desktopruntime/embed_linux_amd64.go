//go:build linux && amd64 && desktop_runtime

package desktopruntime

import _ "embed"

const archiveName = "desktop-runtime-linux-amd64.tar.zst"

//go:embed embed/desktop-runtime-linux-amd64.tar.zst
var archive []byte
