package health_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/healthcheck/health"

	"github.com/stretchr/testify/assert"
)

func Test_MessageURL(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		code     health.Code
		base     string
		expected string
	}{
		{"empty", "", "", "https://coder.com/docs/admin/monitoring/health-check#eunknown"},
		{"default", health.CodeAccessURLFetch, "", "https://coder.com/docs/admin/monitoring/health-check#eacs03"},
		{"custom docs base", health.CodeAccessURLFetch, "https://example.com/docs", "https://example.com/docs/admin/monitoring/health-check#eacs03"},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uut := health.Message{Code: tt.code}
			actual := uut.URL(tt.base)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
