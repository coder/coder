import type { Meta, StoryObj } from "@storybook/react";
import { Avatar } from "components/Avatar/Avatar";
import {
	CalendarCogIcon,
	FingerprintIcon,
	KeyIcon,
	LockIcon,
	UserIcon,
} from "lucide-react";
import { Sidebar, SidebarHeader, SidebarNavItem } from "./Sidebar";

const meta: Meta<typeof Sidebar> = {
	title: "components/Sidebar",
	component: Sidebar,
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const Default: Story = {
	args: {
		children: (
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
	},
};
