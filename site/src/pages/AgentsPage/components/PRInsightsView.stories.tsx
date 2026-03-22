import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import dayjs from "dayjs";
import { useState } from "react";
import { type PRInsightsTimeRange, PRInsightsView } from "./PRInsightsView";

// ---------------------------------------------------------------------------
// Mock data generators
// ---------------------------------------------------------------------------

const NOW = dayjs("2025-07-15");

// Deterministic PRNG so story snapshots are stable across runs.
function seededRandom(seed: number) {
	let s = seed;
	return () => {
		s = (s * 16807) % 2147483647;
		return (s - 1) / 2147483646;
	};
}

function generateTimeSeries(
	days: number,
	opts: { avgCreated: number; avgMerged: number; avgClosed?: number },
	seed = 42,
): TypesGen.PRInsightsTimeSeriesEntry[] {
	const rand = seededRandom(seed);
	const entries: TypesGen.PRInsightsTimeSeriesEntry[] = [];
	for (let i = days - 1; i >= 0; i--) {
		const date = NOW.subtract(i, "day").format("YYYY-MM-DD");
		const jitter = () => Math.round((rand() - 0.3) * 3);
		const created = Math.max(0, opts.avgCreated + jitter());
		const merged = Math.min(created, Math.max(0, opts.avgMerged + jitter()));
		const closed = Math.max(
			0,
			(opts.avgClosed ?? 0) + Math.round((rand() - 0.5) * 2),
		);
		entries.push({
			date,
			prs_created: created,
			prs_merged: merged,
			prs_closed: closed,
		});
	}
	return entries;
}

const MODELS: TypesGen.PRInsightsModelBreakdown[] = [
	{
		model_config_id: "m1",
		display_name: "Claude Sonnet 4",
		provider: "Anthropic",
		total_prs: 48,
		merged_prs: 35,
		merge_rate: 0.73,
		total_additions: 8420,
		total_deletions: 2130,
		total_cost_micros: 142_000_000,
		cost_per_merged_pr_micros: 4_057_143,
	},
	{
		model_config_id: "m2",
		display_name: "GPT-4.1",
		provider: "OpenAI",
		total_prs: 31,
		merged_prs: 20,
		merge_rate: 0.645,
		total_additions: 5100,
		total_deletions: 1340,
		total_cost_micros: 98_400_000,
		cost_per_merged_pr_micros: 4_920_000,
	},
	{
		model_config_id: "m3",
		display_name: "Gemini 2.5 Pro",
		provider: "Google",
		total_prs: 18,
		merged_prs: 14,
		merge_rate: 0.778,
		total_additions: 3200,
		total_deletions: 890,
		total_cost_micros: 41_300_000,
		cost_per_merged_pr_micros: 2_950_000,
	},
	{
		model_config_id: "m4",
		display_name: "Claude Opus 4",
		provider: "Anthropic",
		total_prs: 8,
		merged_prs: 7,
		merge_rate: 0.875,
		total_additions: 2100,
		total_deletions: 480,
		total_cost_micros: 64_200_000,
		cost_per_merged_pr_micros: 9_171_429,
	},
];

const PR_TITLES = [
	"fix: resolve race condition in workspace agent reconnect",
	"feat: add OAuth2 PKCE support for external apps",
	"refactor: extract provisioner job queue into separate package",
	"fix: correct RBAC check for template version imports",
	"feat: add workspace build timeline visualization",
	"chore: upgrade Go to 1.24 and update dependencies",
	"fix: handle nil pointer in DERP mesh coordinator",
	"feat: implement workspace dormancy auto-deletion policy",
	"fix: prevent duplicate agent stats insertion on restart",
	"feat: add audit log entries for SSH connections",
	"refactor: simplify template parameter validation logic",
	"fix: correct timezone handling in usage stats rollup",
	"feat: add support for workspace agent environment variables",
	"fix: resolve flaky TestWorkspaceBuild integration test",
	"feat: implement organization-scoped template policies",
];

const AUTHORS = [
	{
		login: "kylecarbs",
		avatar: "https://avatars.githubusercontent.com/u/7122116",
	},
	{
		login: "ammario",
		avatar: "https://avatars.githubusercontent.com/u/9078713",
	},
	{
		login: "mafredri",
		avatar: "https://avatars.githubusercontent.com/u/147409",
	},
	{
		login: "aslilac",
		avatar: "https://avatars.githubusercontent.com/u/23068824",
	},
	{
		login: "sreya",
		avatar: "https://avatars.githubusercontent.com/u/67369800",
	},
	{
		login: "mtojek",
		avatar: "https://avatars.githubusercontent.com/u/14044910",
	},
	{
		login: "deansheather",
		avatar: "https://avatars.githubusercontent.com/u/11241812",
	},
];

function generatePRs(
	count: number,
	seed = 100,
): TypesGen.PRInsightsPullRequest[] {
	const rand = seededRandom(seed);
	const states: Array<"open" | "closed" | "merged"> = [
		"merged",
		"merged",
		"merged",
		"merged",
		"merged",
		"open",
		"open",
		"closed",
	];
	const models = [
		"Claude Sonnet 4",
		"GPT-4.1",
		"Gemini 2.5 Pro",
		"Claude Opus 4",
	];

	return Array.from({ length: count }, (_, i) => {
		const state = states[i % states.length];
		const author = AUTHORS[i % AUTHORS.length];
		const additions = Math.round(40 + rand() * 400);
		const deletions = Math.round(10 + rand() * 150);

		return {
			chat_id: `chat-${i}`,
			pr_title: PR_TITLES[i % PR_TITLES.length],
			pr_url: `https://github.com/coder/coder/pull/${1200 + i}`,
			pr_number: 1200 + i,
			state,
			draft: state === "open" && i % 3 === 0,
			additions,
			deletions,
			changed_files: Math.round(2 + rand() * 12),
			commits: Math.round(1 + rand() * 6),
			approved:
				state === "merged" ? true : state === "open" ? undefined : false,
			changes_requested: state === "closed" && i % 2 === 0,
			reviewer_count:
				state === "merged"
					? Math.round(1 + rand() * 2)
					: Math.round(rand() * 2),
			author_login: author.login,
			author_avatar_url: author.avatar,
			base_branch: "main",
			model_display_name: models[i % models.length],
			cost_micros: Math.round(1_500_000 + rand() * 8_000_000),
			created_at: NOW.subtract(
				i * 4 + Math.round(rand() * 8),
				"hour",
			).toISOString(),
		};
	});
}

// ---------------------------------------------------------------------------
// Assembled mock datasets
// ---------------------------------------------------------------------------

const defaultData: TypesGen.PRInsightsResponse = {
	summary: {
		total_prs_created: 105,
		total_prs_merged: 76,
		merge_rate: 0.724,
		total_additions: 18820,
		total_deletions: 4840,
		total_cost_micros: 346_000_000,
		cost_per_merged_pr_micros: 4_552_632,
		approval_rate: 0.88,
		prev_total_prs_created: 82,
		prev_total_prs_merged: 55,
		prev_merge_rate: 0.671,
		prev_cost_per_merged_pr_micros: 5_120_000,
	},
	time_series: generateTimeSeries(30, {
		avgCreated: 4,
		avgMerged: 3,
		avgClosed: 1,
	}),
	by_model: MODELS,
	recent_prs: generatePRs(12),
};

const highPerformanceData: TypesGen.PRInsightsResponse = {
	summary: {
		total_prs_created: 210,
		total_prs_merged: 189,
		merge_rate: 0.9,
		total_additions: 42_600,
		total_deletions: 11_200,
		total_cost_micros: 520_000_000,
		cost_per_merged_pr_micros: 2_751_323,
		approval_rate: 0.95,
		prev_total_prs_created: 140,
		prev_total_prs_merged: 112,
		prev_merge_rate: 0.8,
		prev_cost_per_merged_pr_micros: 3_400_000,
	},
	time_series: generateTimeSeries(30, {
		avgCreated: 7,
		avgMerged: 6,
		avgClosed: 1,
	}),
	by_model: MODELS.map((m) => ({
		...m,
		merge_rate: Math.min(m.merge_rate + 0.12, 0.98),
		total_prs: m.total_prs * 2,
		merged_prs: Math.round(m.merged_prs * 2.4),
	})),
	recent_prs: generatePRs(15),
};

const lowVolumeData: TypesGen.PRInsightsResponse = {
	summary: {
		total_prs_created: 8,
		total_prs_merged: 3,
		merge_rate: 0.375,
		total_additions: 620,
		total_deletions: 180,
		total_cost_micros: 18_000_000,
		cost_per_merged_pr_micros: 6_000_000,
		approval_rate: 0.67,
		prev_total_prs_created: 12,
		prev_total_prs_merged: 7,
		prev_merge_rate: 0.583,
		prev_cost_per_merged_pr_micros: 4_200_000,
	},
	time_series: generateTimeSeries(30, { avgCreated: 0, avgMerged: 0 }),
	by_model: MODELS.slice(0, 2).map((m, i) => ({
		...m,
		total_prs: Math.round(m.total_prs / 6),
		merged_prs: Math.round(m.merged_prs / 8),
		merge_rate: 0.35 + i * 0.07,
	})),
	recent_prs: generatePRs(5),
};

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

const meta: Meta<typeof PRInsightsView> = {
	title: "pages/AgentsPage/PRInsightsView",
	component: PRInsightsView,
	decorators: [
		(Story) => (
			<div className="bg-surface-primary p-6" style={{ maxWidth: 1100 }}>
				<Story />
			</div>
		),
	],
	render: function Render(args) {
		const [timeRange, setTimeRange] = useState<PRInsightsTimeRange>(
			args.timeRange,
		);
		return (
			<PRInsightsView
				{...args}
				timeRange={timeRange}
				onTimeRangeChange={setTimeRange}
			/>
		);
	},
};

export default meta;
type Story = StoryObj<typeof PRInsightsView>;

export const Default: Story = {
	args: {
		data: defaultData,
		timeRange: "30d",
		onTimeRangeChange: () => {},
	},
};

export const HighPerformance: Story = {
	args: {
		data: highPerformanceData,
		timeRange: "30d",
		onTimeRangeChange: () => {},
	},
};

export const LowVolume: Story = {
	args: {
		data: lowVolumeData,
		timeRange: "14d",
		onTimeRangeChange: () => {},
	},
};

export const NoPRs: Story = {
	args: {
		data: {
			summary: {
				total_prs_created: 0,
				total_prs_merged: 0,
				merge_rate: 0,
				total_additions: 0,
				total_deletions: 0,
				total_cost_micros: 0,
				cost_per_merged_pr_micros: 0,
				approval_rate: 0,
				prev_total_prs_created: 0,
				prev_total_prs_merged: 0,
				prev_merge_rate: 0,
				prev_cost_per_merged_pr_micros: 0,
			},
			time_series: generateTimeSeries(30, {
				avgCreated: 0,
				avgMerged: 0,
				avgClosed: 0,
			}),
			by_model: [],
			recent_prs: [],
		},
		timeRange: "30d",
		onTimeRangeChange: () => {},
	},
};
