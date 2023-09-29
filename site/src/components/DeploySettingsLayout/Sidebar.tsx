import Brush from "@mui/icons-material/Brush";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import ApprovalIcon from "@mui/icons-material/VerifiedUserOutlined";
import LockRounded from "@mui/icons-material/LockOutlined";
import Globe from "@mui/icons-material/PublicOutlined";
import HubOutlinedIcon from "@mui/icons-material/HubOutlined";
import VpnKeyOutlined from "@mui/icons-material/VpnKeyOutlined";
import MonitorHeartOutlined from "@mui/icons-material/MonitorHeartOutlined";
import { GitIcon } from "components/Icons/GitIcon";
import { Stack } from "components/Stack/Stack";
import type { ElementType, FC, PropsWithChildren, ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { useTheme } from "@mui/system";
import { css } from "@emotion/css";

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
      background-color: ${theme.palette.secondary.dark};
      border-top-left-radius: ${theme.shape.borderRadius};
      border-bottom-left-radius: ${theme.shape.borderRadius};
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
        padding: ${theme.spacing(1.5, 1.5, 1.5, 2)};
        border-radius: ${theme.shape.borderRadius / 2};
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
  const theme = useTheme();
  return (
    <Icon
      css={{
        width: theme.spacing(2),
        height: theme.spacing(2),
      }}
    />
  );
};

export const Sidebar: React.FC = () => {
  const dashboard = useDashboard();

  return (
    <nav
      css={{
        width: 245,
      }}
    >
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
        href="gitauth"
        icon={<SidebarNavItemIcon icon={GitIcon} />}
      >
        Git Authentication
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
