import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	CalendarCogIcon,
	FingerprintIcon,
	KeyIcon,
	LockIcon,
	UserIcon,
} from "lucide-react";
import { Outlet } from "react-router";
import { expect, waitFor, within } from "storybook/test";
import { Avatar } from "#/components/Avatar/Avatar";
import { Sidebar, SidebarHeader, SidebarNavItem } from "./Sidebar";

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
			<SidebarHeader
				avatar={<Avatar fallback="Jon" />}
				title="Jon"
				subtitle="jon@coder.com"
			/>
			<SidebarNavItem href="account" icon={UserIcon}>
				Account
			</SidebarNavItem>
			<SidebarNavItem href="schedule" icon={CalendarCogIcon}>
				Schedule
			</SidebarNavItem>
			<SidebarNavItem href="security" icon={LockIcon}>
				Security
			</SidebarNavItem>
			<SidebarNavItem href="ssh-keys" icon={FingerprintIcon}>
				SSH Keys
			</SidebarNavItem>
			<SidebarNavItem href="tokens" icon={KeyIcon}>
				Tokens
			</SidebarNavItem>
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

export const MobileFullWidth: Story = {
	...Default,
	decorators: [
		(Story) => (
			<div className="flex flex-col gap-2 lg:flex-row">
				<Story />
				<Outlet />
			</div>
		),
	],
	parameters: {
		...Default.parameters,
		// Pixel captures at desktop width by default, where the sidebar is
		// exactly 240px and the play assertion cannot hold.
		pixel: { matrix: { viewports: ["phone"] } },
	},
	// Storybook 10 applies viewports through globals; the legacy
	// parameters.viewport.defaultViewport is only honored by addon-vitest.
	globals: { viewport: { value: "mobile1" } },
	play: async ({ canvasElement }) => {
		// Width is the behavior under test: below lg the sidebar must span
		// the container instead of the fixed 240px desktop column.
		const nav = within(canvasElement).getByRole("navigation");
		await waitFor(() => {
			expect(nav.getBoundingClientRect().width).toBeGreaterThan(240);
		});
	},
};
