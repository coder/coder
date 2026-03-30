import type { Meta, StoryObj } from "@storybook/react-vite";
import { Outlet } from "react-router";
import { userChatProviderConfigsKey } from "#/api/queries/chats";
import { MockUserOwner } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { Sidebar } from "./Sidebar";

const meta = {
	title: "pages/UserSettingsPage/Sidebar",
	component: Sidebar,
	args: {
		user: MockUserOwner,
	},
	decorators: [
		withDashboardProvider,
		(Story) => (
			<div className="flex gap-2">
				<Story />
				<Outlet />
			</div>
		),
	],
	parameters: {
		reactRouter: {
			location: {
				path: "/account",
			},
			routing: [
				{
					path: "/",
					useStoryElement: true,
					children: [
						{ path: "account", element: <>Account page</> },
						{ path: "providers", element: <>Providers page</> },
					],
				},
			],
		},
	},
} satisfies Meta<typeof Sidebar>;

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const WithAgentsEnabled: Story = {
	parameters: {
		experiments: ["agents"],
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [
					{
						provider_id: "prov-1",
						provider: "openai",
						display_name: "OpenAI",
						has_user_api_key: false,
						has_central_api_key_fallback: false,
					},
				],
			},
		],
	},
};

export const WithAgentsDisabled: Story = {
	parameters: {
		experiments: [],
	},
};
