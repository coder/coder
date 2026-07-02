import type { Meta, StoryObj } from "@storybook/react-vite";
import { useParams } from "react-router";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockPermissions, MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import AgentsPage from "./AgentsPage";

const chatList: TypesGen.Chat[] = [
	{
		...MockChat,
		id: "chat-1",
		title: "First agent",
		status: "completed",
		has_unread: false,
	},
	{
		...MockChat,
		id: "chat-2",
		title: "Second agent",
		status: "completed",
		has_unread: false,
	},
];

const emptyPersonalModelOverrides = {
	enabled: false,
} as TypesGen.UserChatPersonalModelOverridesResponse;

// Probe so the play can assert the route changed without mounting the real
// AgentChatPage and its queries.
const ActiveChatProbe = () => {
	const { agentId } = useParams();
	return <div data-testid="active-chat-id">{agentId ?? ""}</div>;
};

const meta: Meta<typeof AgentsPage> = {
	title: "pages/AgentsPage/AgentsPage",
	component: AgentsPage,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withProxyProvider(),
		withWebSocket,
	],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: MockPermissions,
		webSocket: [],
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: {
				path: "/agents",
				useStoryElement: true,
				children: [{ path: ":agentId", element: <ActiveChatProbe /> }],
			},
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockResolvedValue(chatList);
		spyOn(API.experimental, "getChatModels").mockResolvedValue({
			providers: [],
		});
		spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue([]);
		spyOn(
			API.experimental,
			"getUserChatPersonalModelOverrides",
		).mockResolvedValue(emptyPersonalModelOverrides);
	},
};

export default meta;
type Story = StoryObj<typeof AgentsPage>;

// Regression guard for CODAGT-673: selecting a chat must not refetch the
// chat list.
export const SelectingChatIssuesNoListRequest: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const link = await canvas.findByRole("link", { name: /First agent/ });
		await waitFor(() =>
			expect(API.experimental.getChats).toHaveBeenCalledTimes(1),
		);

		await userEvent.click(link);
		await waitFor(() =>
			expect(canvas.getByTestId("active-chat-id")).toHaveTextContent("chat-1"),
		);

		expect(API.experimental.getChats).toHaveBeenCalledTimes(1);
	},
};

const unreadChatList: TypesGen.Chat[] = [
	{
		...MockChat,
		id: "chat-1",
		title: "Unread agent one",
		status: "running",
		has_unread: true,
	},
	{
		...MockChat,
		id: "chat-2",
		title: "Unread agent two",
		status: "running",
		has_unread: true,
	},
];

export const SelectingUnreadChatLeavesUnreadView: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents", searchParams: { chat_status: "unread" } },
			routing: {
				path: "/agents",
				useStoryElement: true,
				children: [{ path: ":agentId", element: <ActiveChatProbe /> }],
			},
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockResolvedValue(unreadChatList);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const link = await canvas.findByRole("link", { name: /Unread agent one/ });
		await waitFor(() =>
			expect(API.experimental.getChats).toHaveBeenCalledTimes(1),
		);

		await userEvent.click(link);
		await waitFor(() =>
			expect(canvas.getByTestId("active-chat-id")).toHaveTextContent("chat-1"),
		);

		await waitFor(() =>
			expect(
				canvas.queryByRole("link", { name: /Unread agent one/ }),
			).toBeNull(),
		);
		expect(
			canvas.getByRole("link", { name: /Unread agent two/ }),
		).toBeInTheDocument();
		expect(API.experimental.getChats).toHaveBeenCalledTimes(1);
	},
};
