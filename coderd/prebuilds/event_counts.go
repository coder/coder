package prebuilds

import "github.com/coder/coder/v2/coderd/database"

// PrebuildEventCounts holds rolling-window counts for a single preset's
// claim events. Each field gives the count in that window.
type PrebuildEventCounts struct {
	ClaimSucceeded WindowedCounts
	ClaimMissed    WindowedCounts
}

// WindowedCounts holds event counts across fixed time windows.
type WindowedCounts struct {
	Count5m   int64
	Count10m  int64
	Count30m  int64
	Count60m  int64
	Count120m int64
}

func newWindowedCounts(row database.GetPrebuildEventCountsRow) WindowedCounts {
	return WindowedCounts{
		Count5m:   row.Count5m,
		Count10m:  row.Count10m,
		Count30m:  row.Count30m,
		Count60m:  row.Count60m,
		Count120m: row.Count120m,
	}
}
