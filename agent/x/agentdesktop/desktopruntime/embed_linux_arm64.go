//go:build linux && arm64 && desktop_runtime

package desktopruntime

import _ "embed"

const archiveName = "desktop-runtime-linux-arm64.tar.zst"

//go:embed embed/desktop-runtime-linux-arm64.tar.zst
var archive []byte
