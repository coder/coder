package proxyhealth

type ProxyHost struct {
	// Host is the root host of the proxy.
	Host string
	// AppHost is the CSP source for the hosts where apps are hosted. See
	// appurl.ConvertAppHostForCSP.
	AppHost string
}
