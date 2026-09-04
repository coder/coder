import type { AgentTimeReport } from "#/api/typesGenerated";

export const MockAgentTimeNow = new Date("2026-09-04T12:00:00Z");

export const MockAgentTimeOrganizationOneId =
	"11111111-1111-1111-1111-111111111111";
const MockAgentTimeOrganizationTwoId = "22222222-2222-2222-2222-222222222222";
const MockAgentTimeOrganizationThreeId = "33333333-3333-3333-3333-333333333333";
export const MockAgentTimeUserOneId = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const MockAgentTimeUserTwoId = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";
const MockAgentTimeUserThreeId = "cccccccc-cccc-cccc-cccc-cccccccccccc";

const MockAgentTimeBuckets = [
	{
		start_date: "2026-08-30",
		end_date: "2026-08-31",
		agent_time_ms: "18000000",
		partial: false,
		complete: true,
	},
	{
		start_date: "2026-08-31",
		end_date: "2026-09-01",
		agent_time_ms: "25200000",
		partial: false,
		complete: true,
	},
	{
		start_date: "2026-09-01",
		end_date: "2026-09-02",
		agent_time_ms: "28800000",
		partial: false,
		complete: true,
	},
	{
		start_date: "2026-09-02",
		end_date: "2026-09-03",
		agent_time_ms: "36000000",
		partial: false,
		complete: true,
	},
	{
		start_date: "2026-09-03",
		end_date: "2026-09-04",
		agent_time_ms: "43200000",
		partial: false,
		complete: true,
	},
	{
		start_date: "2026-09-04",
		end_date: "2026-09-05",
		agent_time_ms: "28800000",
		partial: true,
		complete: false,
	},
] satisfies AgentTimeReport["buckets"];

export const MockAgentTimeReport: AgentTimeReport = {
	start_date: "2026-08-06",
	end_date: "2026-09-05",
	interval: "day",
	total_agent_time_ms: "180000000",
	buckets: MockAgentTimeBuckets,
	rows: [
		{
			id: MockAgentTimeOrganizationOneId,
			name: "Acme Engineering",
			deleted: false,
			agent_time_ms: "108000000",
		},
		{
			id: MockAgentTimeOrganizationTwoId,
			name: "Sunset Labs",
			deleted: true,
			agent_time_ms: "54000000",
		},
		{
			id: MockAgentTimeOrganizationThreeId,
			name: "Platform Team",
			deleted: false,
			agent_time_ms: "18000000",
		},
	],
	count: 3,
	status: {
		capture_started_at: "2026-08-01T00:00:00Z",
		backfill_complete: true,
		backfill_error: "",
		processed_messages: "4200",
		earliest_date: "2026-08-01",
	},
	historical_notice:
		"Agent time is available from captured aggregates. Deleted messages from before capture may not be recoverable.",
};

export const MockAgentTimeOrganizationReport: AgentTimeReport = {
	...MockAgentTimeReport,
	total_agent_time_ms: "108000000",
	rows: [
		{
			id: MockAgentTimeUserOneId,
			name: "Alice Ng",
			deleted: false,
			agent_time_ms: "72000000",
		},
		{
			id: MockAgentTimeUserTwoId,
			name: "Bob Stone",
			deleted: false,
			agent_time_ms: "27000000",
		},
		{
			id: MockAgentTimeUserThreeId,
			name: "Former teammate",
			deleted: true,
			agent_time_ms: "9000000",
		},
	],
	count: 3,
};

export const MockAgentTimeEmptyReport: AgentTimeReport = {
	...MockAgentTimeReport,
	total_agent_time_ms: "0",
	buckets: MockAgentTimeReport.buckets.map((bucket) => ({
		...bucket,
		agent_time_ms: "0",
		partial: false,
		complete: true,
	})),
	rows: [],
	count: 0,
	historical_notice: "",
};

export const MockAgentTimeBackfillingReport: AgentTimeReport = {
	...MockAgentTimeReport,
	status: {
		...MockAgentTimeReport.status,
		backfill_complete: false,
		processed_messages: "1200",
	},
};

export const MockAgentTimeBackfillFailureReport: AgentTimeReport = {
	...MockAgentTimeReport,
	status: {
		...MockAgentTimeReport.status,
		backfill_complete: false,
		backfill_error:
			"Backfill stopped after the aggregate worker lost its lock.",
	},
};

export const MockAgentTimePartialHistoryReport: AgentTimeReport = {
	...MockAgentTimeReport,
	buckets: [
		{
			start_date: "2026-08-01",
			end_date: "2026-08-02",
			agent_time_ms: null,
			partial: false,
			complete: false,
		},
		...MockAgentTimeReport.buckets,
	],
};

export function mockAgentTimeReportWithCount(count: number): AgentTimeReport {
	return {
		...MockAgentTimeReport,
		count,
	};
}
