import type { Meta, StoryObj } from "@storybook/react-vite";
import type { SummaryPanelProps } from "./SummaryPanel";
import { SummaryPanel } from "./SummaryPanel";

const sampleData: SummaryPanelProps = {
	metadata: {
		createdAt: "Dec 12, 2025: 03:01:09 PM",
		lastUpdatedAt: "May 4, 2026: 09:32:05 PM",
		costDisplay: "$87.45",
		tokens: {
			input: 123_132,
			cached: 2_134_563,
			output: 233,
		},
		model: "claude-opus-4-5-20251101",
		tags: ["backend", "api", "performance", "project name"],
	},
	repo: "repo name goes here",
	prDetails: [
		{
			number: 4847,
			title: "Add per-user rate limit",
			additions: 25,
			deletions: 65,
		},
	],
	prompts: [
		{
			index: 37,
			text: "Verify token bucket decay math with edge cases",
		},
		{
			index: 36,
			text: "Add route-level override config to router",
		},
		{
			index: 35,
			text: "Fix retry-after header injection on 429 response",
		},
	],
	totalPrompts: 37,
	activities: [
		{ text: "Set up Redis TTL key schema for user buckets" },
		{ text: "Added route-level override support in router" },
		{ text: "Wrote retry-after header injection on 429" },
	],
	totalActivities: 15,
	files: [
		{
			path: "ratelimit/bucket.go",
			status: "New",
			additions: 54,
			deletions: 0,
		},
		{
			path: "api/routes.go",
			status: "Edited",
			additions: 115,
			deletions: 0,
		},
		{
			path: "docs/api/rate-limiting.md",
			status: "New",
			additions: 25,
			deletions: 0,
		},
		{
			path: "config/defaults.yaml",
			status: "Edited",
			additions: 30,
			deletions: 1,
		},
		{
			path: "config/defaults.yaml",
			status: "Edited",
			additions: 64,
			deletions: 70,
		},
	],
	totalFiles: 34,
	relatedChats: [
		{
			title: "Investigate API latency spikes",
			reason: "references same files",
		},
		{
			title: "Refactor api/middleware chain",
			reason: "references same files",
		},
		{ title: "Add caching to /v1/users", reason: "shared tags" },
		{ title: "Token bucket research notes", reason: "shared tags" },
	],
};

const meta: Meta<typeof SummaryPanel> = {
	title: "pages/AgentsPage/SummaryPanel",
	component: SummaryPanel,
	decorators: [
		(Story) => (
			<div className="h-[900px] w-[420px] border border-border-default bg-surface-primary">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof SummaryPanel>;

export const Default: Story = {
	args: sampleData,
};

export const WithEditableTags: Story = {
	args: {
		...sampleData,
		onRemoveTag: () => {},
		onEditTags: () => {},
	},
};

export const MultiplePRs: Story = {
	args: {
		...sampleData,
		prDetails: [
			{
				number: 4847,
				title: "Add per-user rate limit",
				additions: 25,
				deletions: 65,
			},
			{
				number: 4848,
				title: "Fix rate limit headers",
				additions: 10,
				deletions: 3,
			},
		],
	},
};

export const NoPRs: Story = {
	args: {
		...sampleData,
		prDetails: [],
		repo: undefined,
	},
};

export const MinimalActivity: Story = {
	args: {
		...sampleData,
		prompts: [{ index: 1, text: "Initial prompt" }],
		totalPrompts: 1,
		activities: [{ text: "Started working" }],
		totalActivities: 1,
		files: [
			{
				path: "main.go",
				status: "New",
				additions: 10,
				deletions: 0,
			},
		],
		totalFiles: 1,
		relatedChats: [],
	},
};
