import ScheduleIcon from "@mui/icons-material/EditCalendarOutlined";
import FingerprintOutlinedIcon from "@mui/icons-material/FingerprintOutlined";
import SecurityIcon from "@mui/icons-material/LockOutlined";
import AccountIcon from "@mui/icons-material/Person";
import VpnKeyOutlined from "@mui/icons-material/VpnKeyOutlined";
import type { Meta, StoryObj } from "@storybook/react";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
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
          avatar={<UserAvatar username="Jon" />}
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
