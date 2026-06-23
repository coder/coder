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

// Renders inside the AgentsPage layout's <Outlet> at /agents/:agentId so the
// play function can confirm that selecting a chat changed the active route
// without depending on the real AgentChatPage and its queries.
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
// chat list. The infinite chat-list query is fetched once on mount; clicking
// a chat changes the :agentId route param, and the read-clearing effect must
// update the cache without invalidating (and thus refetching) the list.
export const SelectingChatIssuesNoListRequest: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for the initial chat-list fetch to populate the sidebar.
		const link = await canvas.findByRole("link", { name: /First agent/ });
		await waitFor(() =>
			expect(API.experimental.getChats).toHaveBeenCalledTimes(1),
		);

		// Selecting a chat navigates to /agents/:agentId.
		await userEvent.click(link);
		await waitFor(() =>
			expect(canvas.getByTestId("active-chat-id")).toHaveTextContent("chat-1"),
		);

		// The selection must not have triggered another chat-list request.
		expect(API.experimental.getChats).toHaveBeenCalledTimes(1);
	},
};
