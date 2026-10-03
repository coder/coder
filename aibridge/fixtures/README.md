These fixtures were created by adding logging middleware to API calls to view the raw requests/responses.

```go
...
opts = append(opts, option.WithMiddleware(LoggingMiddleware))
...

func LoggingMiddleware(req *http.Request, next option.MiddlewareNext) (res *http.Response, err error) {
    reqOut, _ := httputil.DumpRequest(req, true)

    // Forward the request to the next handler
    res, err = next(req)
    fmt.Printf("[req] %s\n", reqOut)

    // Handle stuff after the request
    if err != nil {
        return res, err
    }

    respOut, _ := httputil.DumpResponse(res, true)
    fmt.Printf("[resp] %s\n", respOut)

    return res, err
}
```
