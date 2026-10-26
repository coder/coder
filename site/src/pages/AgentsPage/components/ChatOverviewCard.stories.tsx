import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import type { AgentAttachment } from "./ChatConversation/messageHelpers";
import { type ChatOverview, ChatOverviewCardView } from "./ChatOverviewCard";

const attachment = (name: string, mediaType: string): AgentAttachment => ({
	key: name,
	file: { type: "file", media_type: mediaType, name, file_id: name },
	href: `/api/experimental/chats/files/${name}`,
});

const fullOverview: ChatOverview = {
	cost: { totalMicros: 23_960_000, unpricedRequests: 0 },
	isSubagent: false,
	repos: [
		{
			root: "/home/coder/coder",
			branch: "ci/queue-only-jobs",
			additions: 82,
			deletions: 33,
		},
	],
	pullRequests: [
		{
			...MockChatDiffStatus,
			pr_number: 4821,
			pull_request_title: "ci: queue only the jobs a change needs",
			additions: 120,
			deletions: 41,
		},
	],
	subagents: [
		{ id: "sub-1", title: "Find the flaky queue test", status: "running" },
		{ id: "sub-2", title: "Measure CI cost per job", status: "waiting" },
	],
	attachments: [
		attachment("ci-cost-plan.html", "text/html"),
		attachment("flaky-test-report.md", "text/markdown"),
		attachment("queue-1440.png", "image/png"),
		attachment("queue-1280.png", "image/png"),
	],
};

const meta: Meta<typeof ChatOverviewCardView> = {
	title: "pages/AgentsPage/ChatOverviewCard",
	component: ChatOverviewCardView,
	args: {
		overview: fullOverview,
		onOpenTab: fn(),
	},
	decorators: [
		(Story) => (
			<div className="p-4">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ChatOverviewCardView>;

/** Every section, with a fourth attachment folded into "1 more". */
export const Full: Story = {};

/** A chat with a cost and nothing else to show. */
export const CostOnly: Story = {
	args: {
		overview: {
			...fullOverview,
			repos: [],
			pullRequests: [],
			subagents: [],
			attachments: [],
		},
	},
};

/** A sub-agent chat sees the cost of the whole tree, with unpriced usage. */
export const SubagentCost: Story = {
	args: {
		overview: {
			...fullOverview,
			cost: { totalMicros: 4_120_000, unpricedRequests: 2 },
			isSubagent: true,
			subagents: [],
		},
	},
};

/** Long lists stop at three rows; long names truncate. */
export const ManyRows: Story = {
	args: {
		overview: {
			...fullOverview,
			repos: ["api", "site", "docs", "agent", "provisioner"].map(
				(name, index) => ({
					root: `/home/coder/${name}`,
					branch: `feat/a-very-long-branch-name-for-${name}`,
					additions: (index + 1) * 120,
					deletions: index * 9,
				}),
			),
			subagents: ["running", "error", "requires_action", "waiting"].map(
				(status, index) => ({
					id: `sub-${index}`,
					title: `Sub-agent ${index + 1}`,
					status: status as ChatOverview["subagents"][number]["status"],
				}),
			),
		},
	},
};
