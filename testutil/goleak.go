package testutil

import "go.uber.org/goleak"

// GoleakOptions is a common list of options to pass to goleak. This is useful if there is a known
// leaky function we want to exclude from goleak.
var GoleakOptions []goleak.Option = []goleak.Option{
	// seelog (indirect dependency of dd-trace-go) has a known goroutine leak (https://github.com/cihub/seelog/issues/182)
	// When https://github.com/DataDog/dd-trace-go/issues/2987 is resolved, this can be removed.
	goleak.IgnoreAnyFunction("github.com/cihub/seelog.(*asyncLoopLogger).processQueue"),
	// The lumberjack library is used by by agent and seems to leave
	// goroutines after Close(), fails TestGitSSH tests.
	// https://github.com/natefinch/lumberjack/pull/100
	goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
	goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).mill.func1"),
	// The pq library appears to leave around a goroutine after Close().
	goleak.IgnoreTopFunction("github.com/lib/pq.NewDialListener"),
	// database/sql does not immediately close connections after Close().
	// See: https://github.com/golang/go/issues/50616
	goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
}
