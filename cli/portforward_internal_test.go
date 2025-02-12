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
				{"tcp", noAddr, 8000, 8000},
				{"tcp", noAddr, 8080, 8081},
				{"tcp", noAddr, 9000, 9000},
				{"tcp", noAddr, 9001, 9001},
				{"tcp", noAddr, 9002, 9002},
				{"tcp", noAddr, 9003, 9005},
				{"tcp", noAddr, 9004, 9006},
				{"tcp", noAddr, 10000, 10000},
				{"tcp", noAddr, 4444, 4444},
			},
		},
		{
			name: "TCP IPv4 local",
			args: args{
				tcpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []portForwardSpec{
				{"tcp", ipv4Loopback, 8080, 8081},
			},
		},
		{
			name: "TCP IPv6 local",
			args: args{
				tcpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []portForwardSpec{
				{"tcp", ipv6Loopback, 8080, 8081},
			},
		},
		{
			name: "UDP with port range",
			args: args{
				udpSpecs: []string{"8000,8080-8081"},
			},
			want: []portForwardSpec{
				{"udp", noAddr, 8000, 8000},
				{"udp", noAddr, 8080, 8080},
				{"udp", noAddr, 8081, 8081},
			},
		},
		{
			name: "UDP IPv4 local",
			args: args{
				udpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []portForwardSpec{
				{"udp", ipv4Loopback, 8080, 8081},
			},
		},
		{
			name: "UDP IPv6 local",
			args: args{
				udpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []portForwardSpec{
				{"udp", ipv6Loopback, 8080, 8081},
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
