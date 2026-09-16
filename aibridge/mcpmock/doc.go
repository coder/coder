package mcpmock

//go:generate go tool mockgen -destination ./mcpmock.go -package mcpmock github.com/coder/coder/v2/aibridge/mcp ServerProxier
