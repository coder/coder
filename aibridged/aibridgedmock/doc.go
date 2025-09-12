package aibridgedmock

//go:generate mockgen -destination ./clientmock.go -package aibridgedmock github.com/coder/coder/v2/aibridged DRPCClient
//go:generate mockgen -destination ./poolmock.go -package aibridgedmock github.com/coder/coder/v2/aibridged Pooler
