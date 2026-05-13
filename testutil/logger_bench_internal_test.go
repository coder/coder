package testutil

import (
	"testing"

	"cdr.dev/slog/v3"
)

// BenchmarkDefaultDropDebug exercises the default deny-list with a mix of
// matching and non-matching entries. Used to guard against regressions that
// would make the predicate noticeably slow in the hot test-log path.
func BenchmarkDefaultDropDebug(b *testing.B) {
	entries := []slog.SinkEntry{
		{LoggerNames: []string{"coderd", "servertailnet", "net", "wgengine"}, Message: "magicsock: warning"},
		{LoggerNames: []string{"pubsub"}, Message: "publish"},
		{LoggerNames: []string{"pubsub"}, Message: "subscribe"},
		{LoggerNames: []string{"coderd"}, Message: "GET"},
		{LoggerNames: []string{"coderd", "echo"}, Message: "read archive entry"},
		{LoggerNames: []string{"coderd", "echo"}, Message: "unpacking source archive"},
		{LoggerNames: []string{"coderd", "keyrotator"}, Message: "inserted new key for feature"},
		{LoggerNames: []string{"coderd", "dbrollup"}, Message: "rolling up data"},
		{LoggerNames: []string{"coderd", "acquirer"}, Message: "acquiring job"},
		{LoggerNames: []string{"my", "pkg"}, Message: "something"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	var drop bool
	for i := 0; i < b.N; i++ {
		drop = defaultDropDebug(entries[i%len(entries)])
	}
	_ = drop
}
