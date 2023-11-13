import { css } from "@emotion/css";
import {
  useTheme,
  type CSSObject,
  type Interpolation,
  type Theme,
} from "@emotion/react";
import ScheduleIcon from "@mui/icons-material/TimerOutlined";
import type { Workspace } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { type FC, type PropsWithChildren, type ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";
import GeneralIcon from "@mui/icons-material/SettingsOutlined";
import ParameterIcon from "@mui/icons-material/CodeOutlined";
import { Avatar } from "components/Avatar/Avatar";

const SidebarNavItem: FC<
  PropsWithChildren<{ href: string; icon: ReactNode }>
> = ({ children, href, icon }) => {
  const theme = useTheme();

  const linkStyles = css({
    color: "inherit",
    display: "block",
    fontSize: 14,
    textDecoration: "none",
    padding: "12px 12px 12px 16px",
    borderRadius: 4,
    transition: "background-color 0.15s ease-in-out",
    marginBottom: 1,
    position: "relative",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  });

  const activeLinkStyles = css({
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
      borderTopLeftRadius: 8,
      borderBottomLeftRadius: 8,
    },
  });

  return (
    <NavLink
      end
      to={href}
      className={({ isActive }) =>
        combineClasses([linkStyles, isActive ? activeLinkStyles : undefined])
      }
    >
      <Stack alignItems="center" spacing={1.5} direction="row">
        {icon}
        {children}
      </Stack>
    </NavLink>
  );
};

export const Sidebar: FC<{ username: string; workspace: Workspace }> = ({
  username,
  workspace,
}) => {
  return (
    <nav css={styles.sidebar}>
      <Stack direction="row" alignItems="center" css={styles.workspaceInfo}>
        <Avatar src={workspace.template_icon} variant="square" fitImage />
        <Stack spacing={0} css={styles.workspaceData}>
          <Link css={styles.name} to={`/@${username}/${workspace.name}`}>
            {workspace.name}
          </Link>
          <span css={styles.secondary}>
            {workspace.template_display_name ?? workspace.template_name}
          </span>
        </Stack>
      </Stack>

      <SidebarNavItem
        href=""
        icon={<GeneralIcon css={styles.sidebarItemIcon} />}
      >
        General
      </SidebarNavItem>
      <SidebarNavItem
        href="parameters"
        icon={<ParameterIcon css={styles.sidebarItemIcon} />}
      >
        Parameters
      </SidebarNavItem>
      <SidebarNavItem
        href="schedule"
        icon={<ScheduleIcon css={styles.sidebarItemIcon} />}
      >
        Schedule
      </SidebarNavItem>
    </nav>
  );
};

const styles = {
  sidebar: {
    width: 245,
    flexShrink: 0,
  },
  sidebarItemIcon: {
    width: 16,
    height: 16,
  },
  workspaceInfo: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    marginBottom: 16,
  }),
  workspaceData: {
    overflow: "hidden",
  },
  name: (theme) => ({
    fontWeight: 600,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    color: theme.palette.text.primary,
    textDecoration: "none",
  }),
  secondary: (theme) => ({
    color: theme.palette.text.secondary,
    fontSize: 12,
    overflow: "hidden",
    textOverflow: "ellipsis",
  }),
} satisfies Record<string, Interpolation<Theme>>;
