package aibridgedmock

//go:generate go tool mockgen -destination ./clientmock.go -package aibridgedmock github.com/coder/coder/v2/coderd/aibridged DRPCClient
//go:generate go tool mockgen -destination ./poolmock.go -package aibridgedmock github.com/coder/coder/v2/coderd/aibridged Pooler
