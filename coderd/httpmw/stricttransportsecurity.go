package httpmw

import (
	"fmt"
	"net/http"
	"time"
)

// StrictTransportSecurity will add the strict-transport-security header if enabled.
// This header forces a browser to always use https for the domain after it loads https
// once.
// Meaning: On first load of product.coder.com, they are redirected to https.
// 		On all subsequent loads, the client's local browser forces https. This prevents man in the middle.
//
// This header only makes sense if the app is using tls.
// Full header example
//	Strict-Transport-Security: max-age=63072000;
// nolint:revive
func StrictTransportSecurity(enable bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if enable {
				age := time.Hour * 24 * 365 // 1 year
				rw.Header().Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d", int64(age.Seconds())))
			}

			next.ServeHTTP(rw, r)
		})
	}
}
