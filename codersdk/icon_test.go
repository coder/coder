package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestIconURLValid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		icon    string
		wantErr bool
	}{
		{name: "Empty", icon: "", wantErr: false},
		{name: "RelativePath", icon: "/icon/aws.svg", wantErr: false},
		{name: "EmojiPath", icon: "/emojis/1f4bb.png", wantErr: false},
		{name: "QueryString", icon: "/icon/aws.svg?v=2", wantErr: false},
		{name: "HTTPS", icon: "https://example.com/icon.png", wantErr: true},
		{name: "HTTP", icon: "http://example.com/icon.png", wantErr: true},
		{name: "UppercaseScheme", icon: "HTTPS://example.com/icon.png", wantErr: true},
		{name: "ProtocolRelative", icon: "//example.com/icon.png", wantErr: true},
		{name: "BackslashProtocolRelative", icon: `/\example.com/icon.png`, wantErr: true},
		{name: "JavaScript", icon: "javascript:alert(1)", wantErr: true},
		{name: "Data", icon: "data:image/png;base64,xxx", wantErr: true},
		{name: "MissingLeadingSlash", icon: "icon/aws.svg", wantErr: true},
		{name: "DotDotTraversal", icon: "/icon/../../etc/passwd", wantErr: true},
		{name: "EncodedTraversal", icon: "/icon/%2e%2e/secret", wantErr: true},
		{name: "TrailingSlash", icon: "/icon/", wantErr: true},
		{name: "UserInfo", icon: "//user:pass@example.com/x", wantErr: true},
		{name: "ControlCharacter", icon: "/icon/\x00.png", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.IconURLValid(tc.icon)
			if tc.wantErr {
				require.Error(t, err, "icon %q should be rejected", tc.icon)
			} else {
				require.NoError(t, err, "icon %q should be accepted", tc.icon)
			}
		})
	}
}
