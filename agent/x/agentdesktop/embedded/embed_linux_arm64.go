//go:build portabledesktop_embed

package embedded

import _ "embed"

//go:embed bin/portabledesktop-linux-arm64
var embeddedBinary []byte
