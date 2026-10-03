// Swagger UI requestInterceptor.
//
// Returned to Swagger UI as the value of the `requestInterceptor` config
// option. Swagger UI evaluates this string as a JavaScript expression that
// must produce a function which receives a request object and returns the
// (possibly mutated) request.
//
// `withCredentials: false` should disable fetch sending browser credentials,
// but for whatever reason it does not. So this interceptor explicitly omits
// browser credentials from every request to avoid the cookie auth and the
// header auth competing.
(request => {
	request.credentials = "omit";
	return request;
})
