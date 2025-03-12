package testcases

import "github.com/coder/coder/v2/coderd/database"

type QueuePositionTestCase struct {
	Name           string
	JobTags        []database.StringMap
	DaemonTags     []database.StringMap
	QueueSizes     []int64
	QueuePositions []int64
	// GetProvisionerJobsByIDsWithQueuePosition takes jobIDs as a parameter.
	// If SkipJobIDs is empty, all jobs are passed to the function; otherwise, the specified jobs are skipped.
	// NOTE: Skipping job IDs means they will be excluded from the result,
	// but this should not affect the queue position or queue size of other jobs.
	SkipJobIDs map[int]struct{}
	// Set `RequiresEmptyEnvironment` to true if the test requires an environment
	// without any provisioner daemons or jobs present.
	// In API-level tests, a default provisioner daemon is often included,
	// so we may opt to skip such tests.
	RequiresEmptyEnvironment bool
}

func GetDBLevelQueuePositionTestCases() []QueuePositionTestCase {
	return queuePositionTestCases
}

func GetAPILevelQueuePositionTestCases() []QueuePositionTestCase {
	testCases := make([]QueuePositionTestCase, 0)
	for _, tc := range queuePositionTestCases {
		if tc.RequiresEmptyEnvironment {
			continue
		}

		testCases = append(testCases, tc)
	}

	return testCases
}

// queuePositionTestCases contains shared test cases used across multiple tests.
var queuePositionTestCases = []QueuePositionTestCase{
	// Baseline test case
	{
		Name: "test-case-1",
		JobTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
		},
		QueueSizes:     []int64{0, 2, 2},
		QueuePositions: []int64{0, 1, 1},
	},
	// Includes an additional provisioner
	{
		Name: "test-case-2",
		JobTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "b": "2", "c": "3"},
		},
		QueueSizes:     []int64{3, 3, 3},
		QueuePositions: []int64{3, 1, 1},
	},
	// Skips job at index 0
	{
		Name: "test-case-3",
		JobTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "b": "2", "c": "3"},
		},
		QueueSizes:     []int64{3, 3},
		QueuePositions: []int64{3, 1},
		SkipJobIDs: map[int]struct{}{
			0: {},
		},
	},
	// Skips job at index 1
	{
		Name: "test-case-4",
		JobTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "b": "2", "c": "3"},
		},
		QueueSizes:     []int64{3, 3},
		QueuePositions: []int64{3, 1},
		SkipJobIDs: map[int]struct{}{
			1: {},
		},
	},
	// Skips job at index 2
	{
		Name: "test-case-5",
		JobTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "b": "2", "c": "3"},
		},
		QueueSizes:     []int64{3, 3},
		QueuePositions: []int64{1, 1},
		SkipJobIDs: map[int]struct{}{
			2: {},
		},
	},
	// Skips jobs at indexes 0 and 2
	{
		Name: "test-case-6",
		JobTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "b": "2", "c": "3"},
		},
		QueueSizes:     []int64{3},
		QueuePositions: []int64{1},
		SkipJobIDs: map[int]struct{}{
			0: {},
			2: {},
		},
	},
	// Includes two additional jobs that any provisioner can execute.
	{
		Name: "test-case-7",
		JobTags: []database.StringMap{
			{},
			{},
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "b": "2", "c": "3"},
		},
		QueueSizes:     []int64{5, 5, 5, 5, 5},
		QueuePositions: []int64{5, 3, 3, 2, 1},
	},
	// Includes two additional jobs that any provisioner can execute, but they are intentionally skipped.
	{
		Name: "test-case-8",
		JobTags: []database.StringMap{
			{},
			{},
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "c": "3"},
		},
		DaemonTags: []database.StringMap{
			{"a": "1", "b": "2"},
			{"a": "1"},
			{"a": "1", "b": "2", "c": "3"},
		},
		QueueSizes:     []int64{5, 5, 5},
		QueuePositions: []int64{5, 3, 3},
		SkipJobIDs: map[int]struct{}{
			0: {},
			1: {},
		},
	},
	// N jobs (1 job with 0 tags) & 0 provisioners exist
	{
		Name: "test-case-9",
		JobTags: []database.StringMap{
			{},
			{"a": "1"},
			{"b": "2"},
		},
		DaemonTags:               []database.StringMap{},
		QueueSizes:               []int64{0, 0, 0},
		QueuePositions:           []int64{0, 0, 0},
		RequiresEmptyEnvironment: true,
	},
	// N jobs (1 job with 0 tags) & N provisioners
	{
		Name: "test-case-10",
		JobTags: []database.StringMap{
			{},
			{"a": "1"},
			{"b": "2"},
		},
		DaemonTags: []database.StringMap{
			{},
			{"a": "1"},
			{"b": "2"},
		},
		QueueSizes:     []int64{2, 2, 2},
		QueuePositions: []int64{2, 2, 1},
	},
	// (N + 1) jobs (1 job with 0 tags) & N provisioners
	// 1 job not matching any provisioner (first in the list)
	{
		Name: "test-case-11",
		JobTags: []database.StringMap{
			{"c": "3"},
			{},
			{"a": "1"},
			{"b": "2"},
		},
		DaemonTags: []database.StringMap{
			{},
			{"a": "1"},
			{"b": "2"},
		},
		QueueSizes:     []int64{2, 2, 2, 0},
		QueuePositions: []int64{2, 2, 1, 0},
	},
	// 0 jobs & 0 provisioners
	{
		Name:           "test-case-12",
		JobTags:        []database.StringMap{},
		DaemonTags:     []database.StringMap{},
		QueueSizes:     nil, // TODO(yevhenii): should it be empty array instead?
		QueuePositions: nil,
	},
}
