import GeneralIcon from "@mui/icons-material/SettingsOutlined";
import type { FC } from "react";
import type { Template } from "api/typesGenerated";
import {
  Sidebar as BaseSidebar,
  SidebarNavItem,
} from "components/Sidebar/Sidebar";

interface SidebarProps {
  template: Template;
}

export const Sidebar: FC<SidebarProps> = ({ template }) => {
  return (
    <BaseSidebar>
      <SidebarNavItem href="" icon={GeneralIcon}>
        General
      </SidebarNavItem>
    </BaseSidebar>
  );
};
