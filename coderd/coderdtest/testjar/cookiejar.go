package testjar

import (
	"net/http"
	"net/url"
	"sync"
)

func New() *Jar {
	return &Jar{}
}

// Jar exists because 'cookiejar.New()' strips many of the http.Cookie fields
// that are needed to assert. Such as 'Secure' and 'SameSite'.
type Jar struct {
	m      sync.Mutex
	perURL map[string][]*http.Cookie
}

func (j *Jar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.m.Lock()
	defer j.m.Unlock()
	if j.perURL == nil {
		j.perURL = make(map[string][]*http.Cookie)
	}
	j.perURL[u.Host] = append(j.perURL[u.Host], cookies...)
}

func (j *Jar) Cookies(u *url.URL) []*http.Cookie {
	j.m.Lock()
	defer j.m.Unlock()
	return j.perURL[u.Host]
}
