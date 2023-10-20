import { makeStyles } from "@mui/styles";
import ScheduleIcon from "@mui/icons-material/TimerOutlined";
import VariablesIcon from "@mui/icons-material/CodeOutlined";
import { Template } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { FC, ElementType, PropsWithChildren, ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";
import GeneralIcon from "@mui/icons-material/SettingsOutlined";
import SecurityIcon from "@mui/icons-material/LockOutlined";
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

export const Sidebar: React.FC<{ template: Template }> = ({ template }) => {
  const styles = useStyles();

  return (
    <nav className={styles.sidebar}>
      <Stack
        direction="row"
        alignItems="center"
        className={styles.templateInfo}
      >
        <Avatar src={template.icon} variant="square" fitImage />
        <Stack spacing={0} className={styles.templateData}>
          <Link className={styles.name} to={`/templates/${template.name}`}>
            {template.display_name !== ""
              ? template.display_name
              : template.name}
          </Link>
          <span className={styles.secondary}>{template.name}</span>
        </Stack>
      </Stack>

      <SidebarNavItem href="" icon={<SidebarNavItemIcon icon={GeneralIcon} />}>
        General
      </SidebarNavItem>
      <SidebarNavItem
        href="permissions"
        icon={<SidebarNavItemIcon icon={SecurityIcon} />}
      >
        Permissions
      </SidebarNavItem>
      <SidebarNavItem
        href="variables"
        icon={<SidebarNavItemIcon icon={VariablesIcon} />}
      >
        Variables
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
  templateInfo: {
    ...theme.typography.body2,
    marginBottom: theme.spacing(2),
  },
  templateData: {
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
