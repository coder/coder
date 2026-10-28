import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type { ChatStatus } from "#/api/typesGenerated";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import { type ChatOverview, ChatOverviewCardView } from "./ChatOverviewCard";

const mockAttachment = (
	name: string,
	mediaType: string,
): MessageAttachment => ({
	key: name,
	role: "assistant",
	file: { type: "file", media_type: mediaType, name, file_id: name },
	href: `/api/experimental/chats/files/${name}`,
});

const mockRepo = (
	name: string,
	branch: string,
	additions: number,
	deletions: number,
): ChatOverview["repos"][number] => ({
	root: `/home/coder/${name}`,
	branch,
	additions,
	deletions,
});

const mockChatOverview: ChatOverview = {
	cost: { total_cost_micros: 23_960_000, unpriced_request_count: 0 },
	isSubagent: false,
	repos: [mockRepo("coder", "ci/queue-only-jobs", 82, 33)],
	pullRequests: [
		{
			number: "4821",
			status: {
				...MockChatDiffStatus,
				pull_request_title: "ci: queue only the jobs a change needs",
				additions: 120,
				deletions: 41,
			},
		},
	],
	subagents: [
		{ id: "sub-1", title: "Find the flaky queue test", status: "running" },
		{ id: "sub-2", title: "Measure CI cost per job", status: "waiting" },
	],
	attachments: [
		mockAttachment("ci-cost-plan.html", "text/html"),
		mockAttachment("flaky-test-report.md", "text/markdown"),
		mockAttachment("queue-1440.png", "image/png"),
		mockAttachment("queue-1280.png", "image/png"),
	],
};

const mockEmptyChatOverview: ChatOverview = {
	cost: undefined,
	isSubagent: false,
	repos: [],
	pullRequests: [],
	subagents: [],
	attachments: [],
};

const mockLongChatOverview: ChatOverview = {
	...mockChatOverview,
	repos: [
		mockRepo("api", "feat/a-very-long-branch-name-for-api", 120, 0),
		mockRepo("site", "feat/a-very-long-branch-name-for-site", 240, 9),
		mockRepo("docs", "docs/overview-card", 36, 18),
		mockRepo("agent", "fix/agent-git-watcher", 4, 2),
		mockRepo("provisioner", "main", 1, 1),
	],
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

/** Opened before the chat has anything to show. */
export const Empty: Story = {
	args: { overview: mockEmptyChatOverview },
};

/** Opened while the cost or the messages still load. */
export const Loading: Story = {
	args: { overview: mockEmptyChatOverview, isLoading: true },
};

/** A subagent chat sees the cost of the whole tree, with unpriced usage. */
export const SubagentCost: Story = {
	args: {
		overview: {
			...mockChatOverview,
			cost: { total_cost_micros: 4_120_000, unpriced_request_count: 2 },
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

/**
 * Without AI Gateway the card has no Cost row, and a clean repository still
 * shows its branch with +0 −0.
 */
export const NoCostCleanRepo: Story = {
	args: {
		overview: {
			...mockEmptyChatOverview,
			repos: [mockRepo("coder", "main", 0, 0)],
		},
	},
};

/** Long lists fold behind a toggle, and long names truncate. */
export const ManyRows: Story = {
	args: { overview: mockLongChatOverview },
};

/** The Changes list after its toggle opens. */
export const ManyRowsExpanded: Story = {
	args: { overview: mockLongChatOverview },
	play: async ({ canvasElement }) => {
		const changes = within(canvasElement).getByRole("region", {
			name: "Changes",
		});
		await userEvent.click(
			within(changes).getByRole("button", { name: "Show 2 more" }),
		);
	},
};

const mockBusyStatuses: Partial<Record<number, ChatStatus>> = {
	0: "running",
	16: "running",
	32: "error",
	48: "running",
	64: "requires_action",
	80: "running",
	96: "interrupting",
};

/**
 * A large chat lists 5 of its 7 non-idle subagents, the ones that need the user
 * first, and counts all 112.
 */
export const ManySubagents: Story = {
	args: {
		overview: {
			...mockChatOverview,
			subagents: Array.from({ length: 112 }, (_, index) => ({
				id: `sub-${index}`,
				title: `Subagent ${index + 1}`,
				status: mockBusyStatuses[index] ?? "waiting",
			})),
		},
	},
};

/** Every subagent is idle: the section keeps only its count. */
export const IdleSubagents: Story = {
	args: {
		overview: {
			...mockEmptyChatOverview,
			subagents: mockChatOverview.subagents.map((subagent) => ({
				...subagent,
				status: "waiting",
			})),
		},
	},
};
