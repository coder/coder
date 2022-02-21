package httpmw

// type workspaceResourceContextKey struct{}

// // WorkspaceResource returns the workspace resource from the ExtractWorkspaceResource handler.
// func WorkspaceResource(r *http.Request) database.WorkspaceResource {
// 	user, ok := r.Context().Value(workspaceResourceContextKey{}).(database.WorkspaceResource)
// 	if !ok {
// 		panic("developer error: workspace resource middleware not provided")
// 	}
// 	return user
// }

// // ExtractWorkspaceResource requires authentication using a valid agent token.
// func ExtractWorkspaceResource(db database.Store) func(http.Handler) http.Handler {
// 	return func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
// 			cookie, err := r.Cookie(AuthCookie)
// 			if err != nil {
// 				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
// 					Message: fmt.Sprintf("%q cookie must be provided", AuthCookie),
// 				})
// 				return
// 			}
// 			// TODO: This is really insecure! All authentication should go through
// 			// a shared handler!
// 			resource, err := db.GetWorkspaceResourceByAgentToken(r.Context(), cookie.Value)
// 			if errors.Is(err, sql.ErrNoRows) {
// 				if errors.Is(err, sql.ErrNoRows) {
// 					httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
// 						Message: "agent token is invalid",
// 					})
// 					return
// 				}
// 			}
// 			if err != nil {
// 				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
// 					Message: fmt.Sprintf("get workspace resource: %s", err),
// 				})
// 				return
// 			}

// 			ctx := context.WithValue(r.Context(), workspaceResourceContextKey{}, resource)
// 			next.ServeHTTP(rw, r.WithContext(ctx))
// 		})
// 	}
// }
