package fositeprovider

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpmw"
)

// @Router /oauth2/device/code [post]
func (p *Provider) PostDevice(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	request, err := p.provider.NewDeviceRequest(ctx, r)
	if err != nil {
		p.provider.WriteAccessError(ctx, rw, request, err)
		return
	}

	ua := httpmw.APIKey(r)
	session := p.newSession(ua)

	resp, err := p.provider.NewDeviceResponse(ctx, request, session)
	if err != nil {
		p.provider.WriteAccessError(ctx, rw, request, err)
		return
	}

	p.provider.WriteDeviceResponse(ctx, rw, request, resp)
}

// GET /activate  -> show a simple form asking for user_code
func (p *Provider) ActivateGET() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
      <form method="post" action="/activate">
        <label>Enter code:</label>
        <input name="user_code" />
        <button type="submit">Continue</button>
      </form>
    `))
	}
}

// POST /activate -> verify session login, look up user_code, approve or deny
func (p *Provider) ActivatePOST() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		code := r.Form.Get("user_code")

		ua := httpmw.APIKey(r)

		// Mark approved (or call DenyDeviceCode on user choice)
		if err := p.storage.ApproveDeviceCode(r.Context(), code, ua.UserID); err != nil {
			http.Error(w, "invalid or expired code", 400)
			return
		}

		// Optionally show a “success, you can return to your device” page.
		http.Redirect(w, r, "/activate/success", http.StatusSeeOther)
	}
}
