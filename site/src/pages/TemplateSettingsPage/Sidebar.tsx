import ScheduleIcon from "@mui/icons-material/TimerOutlined";
import VariablesIcon from "@mui/icons-material/CodeOutlined";
import GeneralIcon from "@mui/icons-material/SettingsOutlined";
import SecurityIcon from "@mui/icons-material/LockOutlined";
import { type FC } from "react";
import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
  Sidebar as BaseSidebar,
  SidebarHeader,
  SidebarNavItem,
} from "components/Sidebar/Sidebar";

interface SidebarProps {
  template: Template;
}

export const Sidebar: FC<SidebarProps> = ({ template }) => {
  return (
    <BaseSidebar>
      <SidebarHeader
        avatar={<Avatar src={template.icon} variant="square" fitImage />}
        title={template.display_name || template.name}
        linkTo={`/templates/${template.name}`}
        subtitle={template.name}
      />

      <SidebarNavItem href="" icon={GeneralIcon}>
        General
      </SidebarNavItem>
      <SidebarNavItem href="permissions" icon={SecurityIcon}>
        Permissions
      </SidebarNavItem>
      <SidebarNavItem href="variables" icon={VariablesIcon}>
        Variables
      </SidebarNavItem>
      <SidebarNavItem href="schedule" icon={ScheduleIcon}>
        Schedule
      </SidebarNavItem>
    </BaseSidebar>
  );
};
