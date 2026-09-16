import type { Meta, StoryObj } from "@storybook/react-vite";
import { Outlet } from "react-router";
import { SettingsSidebarNavItem, Sidebar } from "./Sidebar";

const meta: Meta<typeof Sidebar> = {
	title: "components/Sidebar",
	component: Sidebar,
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const Default: Story = {
	decorators: [
		(Story) => {
			return (
				<div className="flex gap-2">
					<Story />
					<Outlet />
				</div>
			);
		},
	],
	render: () => (
		<Sidebar>
			<div className="flex flex-col gap-1">
				<SettingsSidebarNavItem href="account">Account</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="schedule">
					Schedule
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="security">
					Security
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="ssh-keys">
					SSH Keys
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="tokens">Tokens</SettingsSidebarNavItem>
			</div>
		</Sidebar>
	),
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
						{
							path: "account",
							element: <>Account page</>,
						},
						{
							path: "schedule",
							element: <>Schedule page</>,
						},
						{
							path: "security",
							element: <>Security page</>,
						},
						{
							path: "ssh-keys",
							element: <>SSH Keys</>,
						},
						{
							path: "tokens",
							element: <>Tokens page</>,
						},
					],
				},
			],
		},
	},
};
