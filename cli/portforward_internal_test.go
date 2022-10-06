package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parsePortForwards(t *testing.T) {
	t.Parallel()

	portForwardSpecToString := func(v []portForwardSpec) (out []string) {
		for _, p := range v {
			require.Equal(t, p.listenNetwork, p.dialNetwork)
			out = append(out, fmt.Sprintf("%s:%s", strings.Replace(p.listenAddress, "127.0.0.1:", "", 1), strings.Replace(p.dialAddress, "127.0.0.1:", "", 1)))
		}
		return out
	}
	type args struct {
		tcpSpecs []string
		udpSpecs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "TCP mixed ports and ranges",
			args: args{
				tcpSpecs: []string{
					"8000,8080:8081,9000-9002,9003-9004:9005-9006",
					"10000",
				},
			},
			want: []string{
				"8000:8000",
				"8080:8081",
				"9000:9000",
				"9001:9001",
				"9002:9002",
				"9003:9005",
				"9004:9006",
				"10000:10000",
			},
		},
		{
			name: "UDP with port range",
			args: args{
				udpSpecs: []string{"8000,8080-8081"},
			},
			want: []string{
				"8000:8000",
				"8080:8080",
				"8081:8081",
			},
		},
		{
			name: "Bad port range",
			args: args{
				tcpSpecs: []string{"8000-7000"},
			},
			wantErr: true,
		},
		{
			name: "Bad dest port range",
			args: args{
				tcpSpecs: []string{"8080-8081:9080-9082"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePortForwards(tt.args.tcpSpecs, tt.args.udpSpecs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePortForwards() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotStrings := portForwardSpecToString(got)
			require.Equal(t, tt.want, gotStrings)
		})
	}
}
