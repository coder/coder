import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import {
	type ChatOverview,
	ChatOverviewCardView,
} from "./ChatOverviewCardView";

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
	name,
	branch,
	additions,
	deletions,
});

const mockDetachedRepo = mockRepo("provisioner", "", 1, 1);

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
	attachments: [],
};

const mockLongChatOverview: ChatOverview = {
	...mockChatOverview,
	repos: [
		mockRepo("api", "feat/a-very-long-branch-name-for-api", 120, 0),
		mockRepo("site", "feat/a-very-long-branch-name-for-site", 240, 9),
		mockDetachedRepo,
		mockRepo("docs", "docs/overview-card", 36, 18),
		mockRepo("agent", "fix/agent-git-watcher", 4, 2),
	],
};

const meta: Meta<typeof ChatOverviewCardView> = {
	title: "pages/AgentsPage/ChatOverviewCardView",
	component: ChatOverviewCardView,
	args: {
		overview: mockChatOverview,
		onOpenTarget: fn(),
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

export const Full: Story = {};

export const Empty: Story = {
	args: { overview: mockEmptyChatOverview },
};

export const Loading: Story = {
	args: { overview: mockEmptyChatOverview, isLoading: true },
};

export const SubagentCost: Story = {
	args: {
		overview: {
			...mockChatOverview,
			cost: { total_cost_micros: 4_120_000, unpriced_request_count: 2 },
			isSubagent: true,
		},
	},
};

export const CostUnavailable: Story = {
	args: {
		overview: { ...mockChatOverview, cost: "unavailable" },
	},
};

export const NoCostCleanRepo: Story = {
	args: {
		overview: {
			...mockEmptyChatOverview,
			repos: [mockRepo("coder", "main", 0, 0)],
		},
	},
};

export const TruncatedRowTooltip: Story = {
	args: { overview: mockLongChatOverview },
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", {
				name: /^api/,
			}),
		);
	},
};

export const TruncatedRowKeyboardFocus: Story = {
	args: { overview: mockLongChatOverview },
	play: async () => {
		// The Cost row comes first, then the row with the truncated branch.
		await userEvent.tab();
		await userEvent.tab();
	},
};

export const ManyRows: Story = {
	args: { overview: mockLongChatOverview },
};

export const ManyRowsExpanded: Story = {
	args: { overview: mockLongChatOverview },
	play: async ({ canvasElement }) => {
		const repositories = within(canvasElement).getByRole("region", {
			name: "Repositories",
		});
		await userEvent.click(
			within(repositories).getByRole("button", {
				name: "Show all 5 repositories",
			}),
		);
	},
};
