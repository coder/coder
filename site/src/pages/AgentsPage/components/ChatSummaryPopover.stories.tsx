import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { ChatSummaryButton } from "./ChatSummaryPopover";

const chatId = "chat-1";

const mockChat: TypesGen.Chat = {
	id: chatId,
	organization_id: "org-1",
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	title: "Test chat",
	status: "waiting",
	last_turn_summary: null,
	summary: null,
	created_at: "2024-05-01T12:00:00Z",
	updated_at: "2024-05-02T15:30:00Z",
	archived: false,
	shared: false,
	pin_order: 0,
	mcp_server_ids: [],
	labels: {},
	has_unread: false,
	client_type: "ui",
	children: [],
};

const mockCost: TypesGen.ChatCost = {
	root_chat_id: chatId,
	total_cost_micros: 1_250_000,
	priced_message_count: 8,
	unpriced_message_count: 0,
};

type MockRequestOptions = {
	cost?: TypesGen.ChatCost;
	costPending?: boolean;
	summary?: string | null;
};

const mockRequests = ({
	cost = mockCost,
	costPending,
	summary = null,
}: MockRequestOptions = {}) => {
	spyOn(API.experimental, "getChat").mockResolvedValue({
		...mockChat,
		summary,
	});
	if (costPending) {
		spyOn(API.experimental, "getChatCost").mockReturnValue(
			new Promise<TypesGen.ChatCost>(() => undefined),
		);
	} else {
		spyOn(API.experimental, "getChatCost").mockResolvedValue(cost);
	}
};

const openSummary = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(
		canvas.getByRole("button", { name: "Show chat summary" }),
	);
	const body = within(canvasElement.ownerDocument.body);
	await body.findByRole("heading", { name: "Summary" });
	return body;
};

const MobileFrame = (Story: FC) => (
	<div className="w-[390px] max-w-full">
		<Story />
	</div>
);

const meta: Meta<typeof ChatSummaryButton> = {
	title: "pages/AgentsPage/ChatSummaryPopover",
	component: ChatSummaryButton,
	decorators: [withDashboardProvider],
	args: {
		chatId,
	},
};

export default meta;
type Story = StoryObj<typeof ChatSummaryButton>;

export const OpensSummary: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement }) => {
		const body = await openSummary(canvasElement);
		await waitFor(() => {
			expect(body.getByText("Created:")).toBeInTheDocument();
			expect(body.getByText("Updated:")).toBeInTheDocument();
			expect(body.getByText("Cost:")).toBeInTheDocument();
			expect(body.getByText("$1.25")).toBeInTheDocument();
		});
		// chat.summary is null until the background generator produces one, so
		// the popover shows the muted empty state.
		expect(body.getByText("No summary yet.")).toBeInTheDocument();
	},
};

export const WithSummary: Story = {
	beforeEach: () =>
		mockRequests({
			summary:
				"Investigated the flaky CI job, traced it to a cache-layer race, and added a regression test.",
		}),
	play: async ({ canvasElement }) => {
		const body = await openSummary(canvasElement);
		await waitFor(() => {
			expect(
				body.getByText(/traced it to a cache-layer race/),
			).toBeInTheDocument();
		});
		expect(body.queryByText("No summary yet.")).not.toBeInTheDocument();
	},
};

export const CostLoading: Story = {
	beforeEach: () => mockRequests({ costPending: true }),
	play: async ({ canvasElement }) => {
		const body = await openSummary(canvasElement);
		await waitFor(() => {
			expect(body.getByLabelText("Loading cost")).toBeInTheDocument();
		});
	},
};

export const Mobile: Story = {
	decorators: [MobileFrame],
	parameters: {
		chromatic: { viewports: [390] },
	},
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement }) => {
		const body = await openSummary(canvasElement);
		await waitFor(() => {
			expect(body.getByText("Cost:")).toBeInTheDocument();
			expect(body.getByText("$1.25")).toBeInTheDocument();
		});
	},
};
