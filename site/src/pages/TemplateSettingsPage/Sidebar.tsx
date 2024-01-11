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
import { ExternalImage } from "components/ExternalImage/ExternalImage";

interface SidebarProps {
  template: Template;
}

export const Sidebar: FC<SidebarProps> = ({ template }) => {
  return (
    <BaseSidebar>
      <SidebarHeader
        avatar={
          <Avatar variant="square" fitImage>
            <ExternalImage src={template.icon} css={{ width: "100%" }} />
          </Avatar>
        }
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
