import { ScheduleIcon as ScheduleIcon, FingerprintOutlinedIcon as FingerprintOutlinedIcon, SecurityIcon as SecurityIcon, AccountIcon as AccountIcon, VpnKeyOutlined as VpnKeyOutlined } from "lucide-react";
import type { Meta, StoryObj } from "@storybook/react";
import { Avatar } from "components/Avatar/Avatar";
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
				<SidebarNavItem href="account" icon={AccountIcon}>
					Account
				</SidebarNavItem>
				<SidebarNavItem href="schedule" icon={ScheduleIcon}>
					Schedule
				</SidebarNavItem>
				<SidebarNavItem href="security" icon={SecurityIcon}>
					Security
				</SidebarNavItem>
				<SidebarNavItem href="ssh-keys" icon={FingerprintOutlinedIcon}>
					SSH Keys
				</SidebarNavItem>
				<SidebarNavItem href="tokens" icon={VpnKeyOutlined}>
					Tokens
				</SidebarNavItem>
			</Sidebar>
		),
	},
};
