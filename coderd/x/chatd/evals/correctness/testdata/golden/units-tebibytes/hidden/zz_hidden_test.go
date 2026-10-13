package units

import "testing"

func TestHiddenTebibytes(t *testing.T) {
	n, err := ParseSize("2TiB")
	if err != nil || n != 2<<40 {
		t.Fatalf("ParseSize(2TiB) = %d, %v", n, err)
	}
	if got := FormatSize(3 << 40); got != "3.0TiB" {
		t.Fatalf("FormatSize(3TiB) = %q", got)
	}
	if got := FormatSize(1536 << 30); got != "1.5TiB" {
		t.Fatalf("FormatSize(1536GiB) = %q", got)
	}
	if got := FormatSize(512 << 30); got != "512.0GiB" {
		t.Fatalf("FormatSize(512GiB) = %q", got)
	}
}
