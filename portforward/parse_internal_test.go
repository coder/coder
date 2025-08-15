package portforward

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ParseSpecs(t *testing.T) {
	t.Parallel()

	type args struct {
		tcpSpecs []string
		udpSpecs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []Spec
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
			want: []Spec{
				{"tcp", NoAddr, 8000, 8000},
				{"tcp", NoAddr, 8080, 8081},
				{"tcp", NoAddr, 9000, 9000},
				{"tcp", NoAddr, 9001, 9001},
				{"tcp", NoAddr, 9002, 9002},
				{"tcp", NoAddr, 9003, 9005},
				{"tcp", NoAddr, 9004, 9006},
				{"tcp", NoAddr, 10000, 10000},
				{"tcp", NoAddr, 4444, 4444},
			},
		},
		{
			name: "TCP IPv4 local",
			args: args{
				tcpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []Spec{
				{"tcp", IPv4Loopback, 8080, 8081},
			},
		},
		{
			name: "TCP IPv6 local",
			args: args{
				tcpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []Spec{
				{"tcp", IPv6Loopback, 8080, 8081},
			},
		},
		{
			name: "UDP with port range",
			args: args{
				udpSpecs: []string{"8000,8080-8081"},
			},
			want: []Spec{
				{"udp", NoAddr, 8000, 8000},
				{"udp", NoAddr, 8080, 8080},
				{"udp", NoAddr, 8081, 8081},
			},
		},
		{
			name: "UDP IPv4 local",
			args: args{
				udpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []Spec{
				{"udp", IPv4Loopback, 8080, 8081},
			},
		},
		{
			name: "UDP IPv6 local",
			args: args{
				udpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []Spec{
				{"udp", IPv6Loopback, 8080, 8081},
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseSpecs(tt.args.tcpSpecs, tt.args.udpSpecs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSpecs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}
