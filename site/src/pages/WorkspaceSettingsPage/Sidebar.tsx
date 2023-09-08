import { makeStyles } from "@mui/styles";
import ScheduleIcon from "@mui/icons-material/TimerOutlined";
import { Workspace } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { FC, ElementType, PropsWithChildren, ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";
import GeneralIcon from "@mui/icons-material/SettingsOutlined";
import ParameterIcon from "@mui/icons-material/CodeOutlined";
import { Avatar } from "components/Avatar/Avatar";

const SidebarNavItem: FC<
  PropsWithChildren<{ href: string; icon: ReactNode }>
> = ({ children, href, icon }) => {
  const styles = useStyles();
  return (
    <NavLink
      end
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

const SidebarNavItemIcon: React.FC<{ icon: ElementType }> = ({
  icon: Icon,
}) => {
  const styles = useStyles();
  return <Icon className={styles.sidebarNavItemIcon} />;
};

export const Sidebar: React.FC<{ username: string; workspace: Workspace }> = ({
  username,
  workspace,
}) => {
  const styles = useStyles();

  return (
    <nav className={styles.sidebar}>
      <Stack
        direction="row"
        alignItems="center"
        className={styles.workspaceInfo}
      >
        <Avatar src={workspace.template_icon} variant="square" fitImage />
        <Stack spacing={0} className={styles.workspaceData}>
          <Link className={styles.name} to={`/@${username}/${workspace.name}`}>
            {workspace.name}
          </Link>
          <span className={styles.secondary}>
            {workspace.template_display_name ?? workspace.template_name}
          </span>
        </Stack>
      </Stack>

      <SidebarNavItem href="" icon={<SidebarNavItemIcon icon={GeneralIcon} />}>
        General
      </SidebarNavItem>
      <SidebarNavItem
        href="parameters"
        icon={<SidebarNavItemIcon icon={ParameterIcon} />}
      >
        Parameters
      </SidebarNavItem>
      <SidebarNavItem
        href="schedule"
        icon={<SidebarNavItemIcon icon={ScheduleIcon} />}
      >
        Schedule
      </SidebarNavItem>
    </nav>
  );
};

const useStyles = makeStyles((theme) => ({
  sidebar: {
    width: 245,
    flexShrink: 0,
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
  workspaceInfo: {
    ...theme.typography.body2,
    marginBottom: theme.spacing(2),
  },
  workspaceData: {
    overflow: "hidden",
  },
  name: {
    fontWeight: 600,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    color: theme.palette.text.primary,
    textDecoration: "none",
  },
  secondary: {
    color: theme.palette.text.secondary,
    fontSize: 12,
    overflow: "hidden",
    textOverflow: "ellipsis",
  },
}));
