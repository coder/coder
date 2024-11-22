package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parsePortForwards(t *testing.T) {
	t.Parallel()

	type args struct {
		tcpSpecs []string
		udpSpecs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []portForwardSpec
		wantErr bool
	}{
		{
			name: "TCP mixed ports and ranges",
			args: args{
				tcpSpecs: []string{
					"8000,8080:8081,9000-9002,9003-9004:9005-9006",
					"10000",
					"4444-4444",
				},
			},
			want: []portForwardSpec{
				{"tcp", "127.0.0.1:8000", "tcp", "127.0.0.1:8000"},
				{"tcp", "127.0.0.1:8080", "tcp", "127.0.0.1:8081"},
				{"tcp", "127.0.0.1:9000", "tcp", "127.0.0.1:9000"},
				{"tcp", "127.0.0.1:9001", "tcp", "127.0.0.1:9001"},
				{"tcp", "127.0.0.1:9002", "tcp", "127.0.0.1:9002"},
				{"tcp", "127.0.0.1:9003", "tcp", "127.0.0.1:9005"},
				{"tcp", "127.0.0.1:9004", "tcp", "127.0.0.1:9006"},
				{"tcp", "127.0.0.1:10000", "tcp", "127.0.0.1:10000"},
				{"tcp", "127.0.0.1:4444", "tcp", "127.0.0.1:4444"},
			},
		},
		{
			name: "TCP IPv4 local",
			args: args{
				tcpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []portForwardSpec{
				{"tcp", "127.0.0.1:8080", "tcp", "127.0.0.1:8081"},
			},
		},
		{
			name: "TCP IPv6 local",
			args: args{
				tcpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []portForwardSpec{
				{"tcp", "[::1]:8080", "tcp", "127.0.0.1:8081"},
			},
		},
		{
			name: "UDP with port range",
			args: args{
				udpSpecs: []string{"8000,8080-8081"},
			},
			want: []portForwardSpec{
				{"udp", "127.0.0.1:8000", "udp", "127.0.0.1:8000"},
				{"udp", "127.0.0.1:8080", "udp", "127.0.0.1:8080"},
				{"udp", "127.0.0.1:8081", "udp", "127.0.0.1:8081"},
			},
		},
		{
			name: "UDP IPv4 local",
			args: args{
				udpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []portForwardSpec{
				{"udp", "127.0.0.1:8080", "udp", "127.0.0.1:8081"},
			},
		},
		{
			name: "UDP IPv6 local",
			args: args{
				udpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []portForwardSpec{
				{"udp", "[::1]:8080", "udp", "127.0.0.1:8081"},
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
			require.Equal(t, tt.want, got)
		})
	}
}
