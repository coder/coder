// Package boundaryusage tracks workspace boundary usage for telemetry reporting.
//
// Each replica does in-memory usage tracking. Accumulated stats are periodically
// flushed to a database table keyed by replica ID. Telemetry aggregates are computed
// across all replicas when generating snapshots.
//
// Aggregate Precision:
//
// The aggregated stats represent approximate usage over roughly the telemetry
// snapshot interval, not a precise time window. This imprecision arises because:
//
//   - Each replica flushes independently, so their data covers slightly different
//     time ranges (varying by up to the flush interval)
//   - Unflushed in-memory data at snapshot time rolls into the next period
//   - The snapshot captures "data flushed since last reset" rather than "usage
//     during exactly the last N minutes"
//
// We accept this imprecision to keep the architecture simple. Each replica
// operates independently and flushes to the database on their own schedule.
// The only synchronization is a database lock that ensures exactly one replica
// reports telemetry per period.
//
// This approach also minimizes database load. The table contains at most one
// row per replica, so flushes are just upserts, and resets only delete N
// rows. There's no accumulation of historical data to clean up.
//
// For telemetry purposes (tracking trends and rough usage patterns),
// approximate data is acceptable.
//
// Known Shortcomings:
//
//   - Ad-hoc boundary usage in a workspace is not accounted for
//   - Unique workspace/user counts may be inflated when the same workspace or
//     user connects through multiple replicas, as each replica tracks its own
//     unique set
//
// Implementation:
//
// The Tracker maintains sets of unique workspace IDs and user IDs, plus request
// counters. When boundary logs are reported, Track() adds the IDs to the sets
// and increments request counters.
//
// FlushToDB() writes stats to the database, replacing all values with the current
// in-memory state. Stats accumulate in memory throughout the telemetry period.
//
// A new period is detected when the upsert results in an INSERT (meaning
// telemetry deleted the replica's row). At that point, all in-memory stats are
// reset so they only count usage within the new period.
//
// Below is a sequence diagram showing the flow of boundary usage tracking.
//
//	┌───────┐       ┌───────────────┐    ┌──────────┐    ┌────┐    ┌───────────┐
//	│ Agent │       │BoundaryLogsAPI│    │ Tracker  │    │ DB │    │ Telemetry │
//	└───┬───┘       └───────┬───────┘    └────┬─────┘    └──┬─┘    └─────┬─────┘
//	    │                   │                 │             │            │
//	    │ ReportBoundaryLogs│                 │             │            │
//	    ├──────────────────►│                 │             │            │
//	    │                   │                 │             │            │
//	    │                   │  Track(wsID,    │             │            │
//	    │                   │    ownerID,...) │             │            │
//	    │                   ├────────────────►│             │            │
//	    │                   │                 │             │            │
//	    │                   │                 │  Every min  │            │
//	    │                   │                 │  FlushToDB  │            │
//	    │                   │                 ├────────────►│            │
//	    │                   │                 │             │            │
//	    │                   │                 │             │ Snapshot   │
//	    │                   │                 │             │ interval   │
//	    │                   │                 │             │◄───────────┤
//	    │                   │                 │             │ Aggregate  │
//	    │                   │                 │             │ & Reset    │
package boundaryusage
