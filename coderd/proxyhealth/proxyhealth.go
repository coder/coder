package proxyhealth

type ProxyHost struct {
	// Host is the root host of the proxy.
	Host string
	// AppHost is the wildcard host where apps are hosted.
	AppHost string
}
