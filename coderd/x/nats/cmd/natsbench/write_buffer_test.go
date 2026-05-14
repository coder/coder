package main

import "testing"

func TestWriteBufferHeader(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		size int
		want string
	}{
		// Zero is the documented "preserve nats.go default" sentinel
		// and must render no header suffix so legacy runs that don't
		// pass -write-buffer print exactly the same header line they
		// always have.
		{name: "zero produces empty suffix", size: 0, want: ""},
		{name: "32 KiB", size: 32 * 1024, want: " write-buffer=32768"},
		{name: "1 MiB", size: 1 << 20, want: " write-buffer=1048576"},
		{name: "4 MiB", size: 4 << 20, want: " write-buffer=4194304"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := writeBufferHeader(tc.size)
			if got != tc.want {
				t.Fatalf("writeBufferHeader(%d) = %q, want %q", tc.size, got, tc.want)
			}
		})
	}
}
