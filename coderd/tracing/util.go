package tracing

import (
	"runtime"
	"strings"
)

const TracerName = "coderd"

func FuncName() string {
	fnpc, _, _, ok := runtime.Caller(1)
	if !ok {
		return ""
	}
	fn := runtime.FuncForPC(fnpc)
	name := fn.Name()
	if i := strings.LastIndex(name, "/"); i > 0 {
		name = name[i+1:]
	}
	return name
}

func FuncNameSkip(skip int) string {
	fnpc, _, _, ok := runtime.Caller(1 + skip)
	if !ok {
		return ""
	}
	fn := runtime.FuncForPC(fnpc)
	name := fn.Name()
	if i := strings.LastIndex(name, "/"); i > 0 {
		name = name[i+1:]
	}
	return name
}
