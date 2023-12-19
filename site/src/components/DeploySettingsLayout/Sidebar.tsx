import Brush from "@mui/icons-material/Brush";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import ApprovalIcon from "@mui/icons-material/VerifiedUserOutlined";
import LockRounded from "@mui/icons-material/LockOutlined";
import InsertChartIcon from "@mui/icons-material/InsertChart";
import Globe from "@mui/icons-material/PublicOutlined";
import HubOutlinedIcon from "@mui/icons-material/HubOutlined";
import VpnKeyOutlined from "@mui/icons-material/VpnKeyOutlined";
import MonitorHeartOutlined from "@mui/icons-material/MonitorHeartOutlined";
import { type FC } from "react";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { GitIcon } from "components/Icons/GitIcon";
import {
  Sidebar as BaseSidebar,
  SidebarNavItem,
} from "components/Sidebar/Sidebar";

export const Sidebar: FC = () => {
  const dashboard = useDashboard();

  return (
    <BaseSidebar>
      <SidebarNavItem href="general" icon={LaunchOutlined}>
        General
      </SidebarNavItem>
      <SidebarNavItem href="licenses" icon={ApprovalIcon}>
        Licenses
      </SidebarNavItem>
      <SidebarNavItem href="appearance" icon={Brush}>
        Appearance
      </SidebarNavItem>
      <SidebarNavItem href="userauth" icon={VpnKeyOutlined}>
        User Authentication
      </SidebarNavItem>
      <SidebarNavItem href="external-auth" icon={GitIcon}>
        External Authentication
      </SidebarNavItem>
      <SidebarNavItem href="network" icon={Globe}>
        Network
      </SidebarNavItem>
      <SidebarNavItem href="workspace-proxies" icon={HubOutlinedIcon}>
        Workspace Proxies
      </SidebarNavItem>
      <SidebarNavItem href="security" icon={LockRounded}>
        Security
      </SidebarNavItem>
      <SidebarNavItem href="observability" icon={InsertChartIcon}>
        Observability
      </SidebarNavItem>
      {dashboard.experiments.includes("deployment_health_page") && (
        <SidebarNavItem href="/health" icon={MonitorHeartOutlined}>
          Health
        </SidebarNavItem>
      )}
    </BaseSidebar>
  );
};
