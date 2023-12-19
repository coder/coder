package cli

import "testing"

func Test_resolveAgentAbsPath(t *testing.T) {
	t.Parallel()

	type args struct {
		workingDirectory string
		relOrAbsPath     string
		agentOS          string
		local            bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"ok no args", args{}, "", false},
		{"ok only working directory", args{workingDirectory: "/some/path"}, "/some/path", false},
		{"ok with working directory and rel path", args{workingDirectory: "/some/path", relOrAbsPath: "other/path"}, "/some/path/other/path", false},
		{"ok with working directory and abs path", args{workingDirectory: "/some/path", relOrAbsPath: "/other/path"}, "/other/path", false},
		{"ok with no working directory and abs path", args{relOrAbsPath: "/other/path"}, "/other/path", false},

		{"fail tilde", args{relOrAbsPath: "~"}, "", true},
		{"fail tilde with working directory", args{workingDirectory: "/some/path", relOrAbsPath: "~"}, "", true},
		{"fail tilde path", args{relOrAbsPath: "~/some/path"}, "", true},
		{"fail tilde path with working directory", args{workingDirectory: "/some/path", relOrAbsPath: "~/some/path"}, "", true},
		{"fail relative dot with no working directory", args{relOrAbsPath: "."}, "", true},
		{"fail relative with no working directory", args{relOrAbsPath: "some/path"}, "", true},

		{"ok with working directory and rel path on windows", args{workingDirectory: "C:\\some\\path", relOrAbsPath: "other\\path", agentOS: "windows"}, "C:\\some\\path\\other\\path", false},
		{"ok with working directory and abs path on windows", args{workingDirectory: "C:\\some\\path", relOrAbsPath: "C:\\other\\path", agentOS: "windows"}, "C:\\other\\path", false},
		{"ok with no working directory and abs path on windows", args{relOrAbsPath: "C:\\other\\path", agentOS: "windows"}, "C:\\other\\path", false},
		{"ok abs unix path on windows", args{workingDirectory: "C:\\some\\path", relOrAbsPath: "/other/path", agentOS: "windows"}, "\\other\\path", false},
		{"ok rel unix path on windows", args{workingDirectory: "C:\\some\\path", relOrAbsPath: "other/path", agentOS: "windows"}, "C:\\some\\path\\other\\path", false},

		{"fail with no working directory and rel path on windows", args{relOrAbsPath: "other\\path", agentOS: "windows"}, "", true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveAgentAbsPath(tt.args.workingDirectory, tt.args.relOrAbsPath, tt.args.agentOS, tt.args.local)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveAgentAbsPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveAgentAbsPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
