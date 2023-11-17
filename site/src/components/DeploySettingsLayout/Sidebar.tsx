import Brush from "@mui/icons-material/Brush";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import ApprovalIcon from "@mui/icons-material/VerifiedUserOutlined";
import LockRounded from "@mui/icons-material/LockOutlined";
import InsertChartIcon from "@mui/icons-material/InsertChart";
import Globe from "@mui/icons-material/PublicOutlined";
import HubOutlinedIcon from "@mui/icons-material/HubOutlined";
import VpnKeyOutlined from "@mui/icons-material/VpnKeyOutlined";
import MonitorHeartOutlined from "@mui/icons-material/MonitorHeartOutlined";
import { GitIcon } from "components/Icons/GitIcon";
import { Stack } from "components/Stack/Stack";
import type { ElementType, FC, PropsWithChildren, ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";

const SidebarNavItem: FC<
  PropsWithChildren<{ href: string; icon: ReactNode }>
> = ({ children, href, icon }) => {
  const theme = useTheme();

  const activeStyles = css`
    background-color: ${theme.palette.action.hover};

    &::before {
      content: "";
      display: block;
      width: 3px;
      height: 100%;
      position: absolute;
      left: 0;
      top: 0;
      background-color: ${theme.palette.primary.main};
      border-top-left-radius: 8px;
      border-bottom-left-radius: 8px;
    }
  `;

  return (
    <NavLink
      to={href}
      className={({ isActive }) => css`
        ${isActive && activeStyles}

        color: inherit;
        display: block;
        font-size: 14px;
        text-decoration: none;
        padding: 12px 12px 12px 16px;
        border-radius: 4px;
        transition: background-color 0.15s ease-in-out;
        margin-bottom: 1;
        position: relative;

        &:hover {
          background-color: ${theme.palette.action.hover};
        }
      `}
    >
      <Stack alignItems="center" spacing={1.5} direction="row">
        {icon}
        {children}
      </Stack>
    </NavLink>
  );
};

const SidebarNavItemIcon: FC<{ icon: ElementType }> = ({ icon: Icon }) => {
  return <Icon css={{ width: 16, height: 16 }} />;
};

export const Sidebar: React.FC = () => {
  const dashboard = useDashboard();

  return (
    <nav css={{ width: 245 }}>
      <SidebarNavItem
        href="general"
        icon={<SidebarNavItemIcon icon={LaunchOutlined} />}
      >
        General
      </SidebarNavItem>
      <SidebarNavItem
        href="licenses"
        icon={<SidebarNavItemIcon icon={ApprovalIcon} />}
      >
        Licenses
      </SidebarNavItem>
      <SidebarNavItem
        href="appearance"
        icon={<SidebarNavItemIcon icon={Brush} />}
      >
        Appearance
      </SidebarNavItem>
      <SidebarNavItem
        href="userauth"
        icon={<SidebarNavItemIcon icon={VpnKeyOutlined} />}
      >
        User Authentication
      </SidebarNavItem>
      <SidebarNavItem
        href="external-auth"
        icon={<SidebarNavItemIcon icon={GitIcon} />}
      >
        External Authentication
      </SidebarNavItem>
      <SidebarNavItem href="network" icon={<SidebarNavItemIcon icon={Globe} />}>
        Network
      </SidebarNavItem>
      {dashboard.experiments.includes("moons") && (
        <SidebarNavItem
          href="workspace-proxies"
          icon={<SidebarNavItemIcon icon={HubOutlinedIcon} />}
        >
          Workspace Proxies
        </SidebarNavItem>
      )}
      <SidebarNavItem
        href="security"
        icon={<SidebarNavItemIcon icon={LockRounded} />}
      >
        Security
      </SidebarNavItem>
      <SidebarNavItem
        href="observability"
        icon={<SidebarNavItemIcon icon={InsertChartIcon} />}
      >
        Observability
      </SidebarNavItem>
      {dashboard.experiments.includes("deployment_health_page") && (
        <SidebarNavItem
          href="/health"
          icon={<SidebarNavItemIcon icon={MonitorHeartOutlined} />}
        >
          Health
        </SidebarNavItem>
      )}
    </nav>
  );
};
