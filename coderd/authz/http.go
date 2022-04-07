package authz

import "net/http"

func H(obj Resource, act Action, handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// auth
		err := Authorize(nil, obj, act)
		if err != nil {
			//unauth
		}
		handlerFunc.ServeHTTP(w, r)
	}
}
