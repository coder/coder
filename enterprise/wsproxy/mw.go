package wsproxy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

type userTokenKey struct{}

// UserSessionToken returns session token from ExtractSessionTokenMW
func UserSessionToken(r *http.Request) string {
	key, ok := r.Context().Value(userTokenKey{}).(string)
	if !ok {
		panic("developer error: ExtractSessionTokenMW middleware not provided")
	}
	return key
}

func ExtractSessionTokenMW() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			token := httpmw.ApiTokenFromRequest(r)
			if token == "" {
				// TODO: If this is empty, we should attempt to smuggle their
				// 		token from the primary. If the user is not logged in there
				//		they should be redirected to a login page.
				httpapi.Write(r.Context(), rw, http.StatusUnauthorized, codersdk.Response{
					Message: httpmw.SignedOutErrorMessage,
					Detail:  fmt.Sprintf("Cookie %q or query parameter must be provided.", codersdk.SessionTokenCookie),
				})
				return
			}
			ctx := context.WithValue(r.Context(), userTokenKey{}, token)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
