//nolint:testpackage
package codersdk

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestPagination_asRequestOption(t *testing.T) {
	t.Parallel()

	uuid1 := uuid.New()
	type fields struct {
		AfterID uuid.UUID
		Limit   int
		Offset  int
	}
	tests := []struct {
		name   string
		fields fields
		want   url.Values
	}{
		{
			name:   "Test AfterID is set",
			fields: fields{AfterID: uuid1},
			want:   url.Values{"after_id": []string{uuid1.String()}},
		},
		{
			name:   "Test Limit is set",
			fields: fields{Limit: 10},
			want:   url.Values{"limit": []string{"10"}},
		},
		{
			name:   "Test Offset is set",
			fields: fields{Offset: 10},
			want:   url.Values{"offset": []string{"10"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := Pagination{
				AfterID: tt.fields.AfterID,
				Limit:   tt.fields.Limit,
				Offset:  tt.fields.Offset,
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			p.asRequestOption()(req)
			got := req.URL.Query()
			assert.Equal(t, tt.want, got)
		})
	}
}
