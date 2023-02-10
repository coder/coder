package httpmw

// SystemAuthCtx sets the system auth context for the request.
// Use sparingly.
// func SystemAuthCtx(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
// 		ctx := dbauthz.AsSystem(r.Context())
// 		next.ServeHTTP(rw, r.WithContext(ctx))
// 	})
// }
