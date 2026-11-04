package codersdk

// WorkloadHostStats counts admitted workloads retained by a host supervisor.
// Rejected requests are not retained or counted. Pending includes requested,
// reserved, starting, and pending workloads.
type WorkloadHostStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Running   int64 `json:"running"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Canceled  int64 `json:"canceled"`
	Unknown   int64 `json:"unknown"`
}

// WorkloadHistoryEntry includes the original admitted request and its observed
// outcome. Unlike ordinary workload responses, it contains command text and
// environment values and must only be exposed to authorized administrators.
type WorkloadHistoryEntry struct {
	Request  StartHostWorkloadRequest `json:"request"`
	Workload Workload                 `json:"workload"`
}

// WorkloadHostHistory is a page of retained admissions ordered by creation time,
// then UUID, ascending. Pages contain at most 10 entries and may be smaller to
// fit the response size limit. Hosts retain at most 1024 admissions and refuse
// new admissions once full rather than evicting idempotency keys.
type WorkloadHostHistory struct {
	Items []WorkloadHistoryEntry `json:"items"`
	Total int64                  `json:"total"`
	// NextOffset is the offset after this page, not a sentinel. When it equals
	// Total, all currently retained admissions have been returned.
	NextOffset int `json:"next_offset"`
}
