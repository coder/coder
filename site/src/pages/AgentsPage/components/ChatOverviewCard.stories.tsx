import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type { ChatStatus } from "#/api/typesGenerated";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import type { AgentAttachment } from "./ChatConversation/messageHelpers";
import { type ChatOverview, ChatOverviewCardView } from "./ChatOverviewCard";

const mockHtmlAttachment: AgentAttachment = {
	key: "ci-cost-plan.html",
	file: {
		type: "file",
		media_type: "text/html",
		name: "ci-cost-plan.html",
		file_id: "file-1",
	},
	href: "/api/experimental/chats/files/file-1",
};
const mockMarkdownAttachment: AgentAttachment = {
	...mockHtmlAttachment,
	key: "flaky-test-report.md",
	file: {
		...mockHtmlAttachment.file,
		media_type: "text/markdown",
		name: "flaky-test-report.md",
	},
};
const mockImageAttachment: AgentAttachment = {
	...mockHtmlAttachment,
	key: "queue-1440.png",
	file: {
		...mockHtmlAttachment.file,
		media_type: "image/png",
		name: "queue-1440.png",
	},
};
const mockSecondImageAttachment: AgentAttachment = {
	...mockImageAttachment,
	key: "queue-1280.png",
	file: { ...mockImageAttachment.file, name: "queue-1280.png" },
};

const mockChatOverview: ChatOverview = {
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
		mockHtmlAttachment,
		mockMarkdownAttachment,
		mockImageAttachment,
		mockSecondImageAttachment,
	],
};

const mockSubagentStatuses = [
	"running",
	"error",
	"requires_action",
	"waiting",
] satisfies ChatStatus[];

const mockLongChatOverview: ChatOverview = {
	...mockChatOverview,
	repos: [
		{
			root: "/home/coder/api",
			branch: "feat/a-very-long-branch-name-for-api",
			additions: 120,
			deletions: 0,
		},
		{
			root: "/home/coder/site",
			branch: "feat/a-very-long-branch-name-for-site",
			additions: 240,
			deletions: 9,
		},
		{
			root: "/home/coder/docs",
			branch: "docs/overview-card",
			additions: 36,
			deletions: 18,
		},
		{
			root: "/home/coder/agent",
			branch: "fix/agent-git-watcher",
			additions: 4,
			deletions: 2,
		},
		{
			root: "/home/coder/provisioner",
			branch: "main",
			additions: 1,
			deletions: 1,
		},
	],
	subagents: mockSubagentStatuses.map((status, index) => ({
		id: `sub-${index}`,
		title: `Subagent ${index + 1}`,
		status,
	})),
};

const meta: Meta<typeof ChatOverviewCardView> = {
	title: "pages/AgentsPage/ChatOverviewCard",
	component: ChatOverviewCardView,
	args: {
		overview: mockChatOverview,
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

/** Every section, with a fourth attachment behind the toggle. */
export const Full: Story = {};

/** A chat with a cost and nothing else to show. */
export const CostOnly: Story = {
	args: {
		overview: {
			...mockChatOverview,
			repos: [],
			pullRequests: [],
			subagents: [],
			attachments: [],
		},
	},
};

/** A subagent chat sees the cost of the whole tree, with unpriced usage. */
export const SubagentCost: Story = {
	args: {
		overview: {
			...mockChatOverview,
			cost: { totalMicros: 4_120_000, unpricedRequests: 2 },
			isSubagent: true,
			subagents: [],
		},
	},
};

/** The cost request failed. */
export const CostUnavailable: Story = {
	args: {
		overview: { ...mockChatOverview, cost: "unavailable" },
	},
};

/** Long lists stop at three rows; long names truncate. */
export const ManyRows: Story = {
	args: { overview: mockLongChatOverview },
};

/** The Changes list after its toggle opens. */
export const ManyRowsExpanded: Story = {
	args: { overview: mockLongChatOverview },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const changes = canvas.getByRole("region", { name: "Changes" });
		await userEvent.click(
			within(changes).getByRole("button", { name: "Show 2 more" }),
		);
	},
};
