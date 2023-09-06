import { makeStyles } from "@mui/styles";
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
import { ElementType, PropsWithChildren, ReactNode, FC } from "react";
import { NavLink } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";
import { useDashboard } from "components/Dashboard/DashboardProvider";

const SidebarNavItem: FC<
  PropsWithChildren<{ href: string; icon: ReactNode }>
> = ({ children, href, icon }) => {
  const styles = useStyles();
  return (
    <NavLink
      to={href}
      className={({ isActive }) =>
        combineClasses([
          styles.sidebarNavItem,
          isActive ? styles.sidebarNavItemActive : undefined,
        ])
      }
    >
      <Stack alignItems="center" spacing={1.5} direction="row">
        {icon}
        {children}
      </Stack>
    </NavLink>
  );
};

const SidebarNavItemIcon: FC<{ icon: ElementType }> = ({ icon: Icon }) => {
  const styles = useStyles();
  return <Icon className={styles.sidebarNavItemIcon} />;
};

export const Sidebar: React.FC = () => {
  const styles = useStyles();
  const dashboard = useDashboard();

  return (
    <nav className={styles.sidebar}>
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

const useStyles = makeStyles((theme) => ({
  sidebar: {
    width: 245,
  },

  sidebarNavItem: {
    color: "inherit",
    display: "block",
    fontSize: 14,
    textDecoration: "none",
    padding: theme.spacing(1.5, 1.5, 1.5, 2),
    borderRadius: theme.shape.borderRadius / 2,
    transition: "background-color 0.15s ease-in-out",
    marginBottom: 1,
    position: "relative",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  },

  sidebarNavItemActive: {
    backgroundColor: theme.palette.action.hover,

    "&:before": {
      content: '""',
      display: "block",
      width: 3,
      height: "100%",
      position: "absolute",
      left: 0,
      top: 0,
      backgroundColor: theme.palette.secondary.dark,
      borderTopLeftRadius: theme.shape.borderRadius,
      borderBottomLeftRadius: theme.shape.borderRadius,
    },
  },

  sidebarNavItemIcon: {
    width: theme.spacing(2),
    height: theme.spacing(2),
  },
}));
