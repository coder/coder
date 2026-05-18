import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
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
	repo: "coder/coder",
	prDetails: [
		{
			number: 4847,
			title: "Add per-user rate limit",
			url: "https://github.com/coder/coder/pull/4847",
			additions: 25,
			deletions: 65,
		},
	],
	prompts: [
		{
			index: 37,
			text: "Verify token bucket decay math with edge cases",
			messageId: 100,
		},
		{
			index: 36,
			text: "Add route-level override config to router",
			messageId: 98,
		},
		{
			index: 35,
			text: "Fix retry-after header injection on 429 response",
			messageId: 96,
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
			path: "config/overrides.yaml",
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
			chatId: "chat-1",
		},
		{
			title: "Refactor api/middleware chain",
			reason: "references same files",
			chatId: "chat-2",
		},
		{
			title: "Add caching to /v1/users",
			reason: "shared tags",
			chatId: "chat-3",
		},
		{
			title: "Token bucket research notes",
			reason: "shared tags",
			chatId: "chat-4",
		},
	],
	onPromptClick: fn(),
	onRelatedChatClick: fn(),
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
		onRemoveTag: fn(),
		onEditTags: fn(),
	},
};

export const MultiplePRs: Story = {
	args: {
		...sampleData,
		prDetails: [
			{
				number: 25426,
				title: "Replace filter dropdown with rich popover",
				url: "https://github.com/coder/coder/pull/25426",
				additions: 25,
				deletions: 65,
			},
			{
				number: 25425,
				title: "Add search to sidebar chat list",
				url: "https://github.com/coder/coder/pull/25425",
				additions: 10,
				deletions: 3,
			},
		],
	},
};

export const FewPrompts: Story = {
	args: {
		...sampleData,
		metadata: {
			...sampleData.metadata,
			createdAt: "2026-05-18T06:00:29.345Z",
			lastUpdatedAt: "2026-05-18T07:00:29.345Z",
			costDisplay: "$4.82",
			tokens: { input: 48_200, cached: 12_800, output: 9_100 },
			model: "claude-sonnet-4-20250514",
			tags: ["frontend", "sidebar", "ux-review"],
		},
		prDetails: [
			{
				number: 25426,
				title: "Replace filter dropdown with rich popover",
				url: "https://github.com/coder/coder/pull/25426",
				additions: 25,
				deletions: 65,
			},
			{
				number: 25425,
				title: "Add search to sidebar chat list",
				url: "https://github.com/coder/coder/pull/25425",
				additions: 10,
				deletions: 3,
			},
		],
		prompts: [
			{
				index: 1,
				text: "Add a search function to the agents sidebar",
				messageId: 10,
			},
			{
				index: 2,
				text: "Replace the filter dropdown with a rich popover",
				messageId: 20,
			},
			{
				index: 3,
				text: "Make the sidebar resize handle thinner",
				messageId: 30,
			},
		],
		totalPrompts: 3,
		activities: [
			{ text: "Created SearchButton component in FilterDropdown.tsx" },
			{
				text: "Added client-side chat filtering with multi-field match",
			},
			{ text: "Replaced FilterIcon with ListFilterIcon" },
		],
		totalActivities: 5,
		files: [
			{
				path: "site/src/components/FilterDropdown.tsx",
				status: "Edited",
				additions: 45,
				deletions: 12,
			},
			{
				path: "site/src/pages/AgentsPage/AgentsSidebar.tsx",
				status: "Edited",
				additions: 80,
				deletions: 5,
			},
			{
				path: "site/src/components/SearchButton.tsx",
				status: "New",
				additions: 35,
				deletions: 0,
			},
			{
				path: "site/src/components/ResizeHandle.tsx",
				status: "Edited",
				additions: 3,
				deletions: 3,
			},
		],
		totalFiles: 4,
		relatedChats: [],
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
		prompts: [{ index: 1, text: "Initial prompt", messageId: 1 }],
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

/** Tests the minimum panel width (360px) to verify nothing overflows. */
export const NarrowWidth: Story = {
	args: sampleData,
	decorators: [
		(Story) => (
			<div className="h-[900px] w-[360px] border border-border-default bg-surface-primary">
				<Story />
			</div>
		),
	],
};
