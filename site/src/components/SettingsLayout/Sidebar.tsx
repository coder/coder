import { css } from "@emotion/css";
import {
  type CSSObject,
  type Interpolation,
  type Theme,
  useTheme,
} from "@emotion/react";
import VpnKeyOutlined from "@mui/icons-material/VpnKeyOutlined";
import FingerprintOutlinedIcon from "@mui/icons-material/FingerprintOutlined";
import {
  type FC,
  type ComponentType,
  type PropsWithChildren,
  type ReactNode,
} from "react";
import { NavLink } from "react-router-dom";
import AccountIcon from "@mui/icons-material/Person";
import ScheduleIcon from "@mui/icons-material/EditCalendarOutlined";
import SecurityIcon from "@mui/icons-material/LockOutlined";
import type { User } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { combineClasses } from "utils/combineClasses";

const SidebarNavItem: FC<
  PropsWithChildren<{ href: string; icon: ReactNode }>
> = ({ children, href, icon }) => {
  const theme = useTheme();

  const sidebarNavItemStyles = css`
    color: inherit;
    display: block;
    font-size: 14px;
    text-decoration: none;
    padding: ${theme.spacing(1.5, 1.5, 1.5, 2)};
    border-radius: ${theme.shape.borderRadius / 2}px;
    transition: background-color 0.15s ease-in-out;
    margin-bottom: 1px;
    position: relative;

    &:hover {
      background-color: theme.palette.action.hover;
    }
  `;

  const sidebarNavItemActiveStyles = css`
    background-color: ${theme.palette.action.hover};

    &:before {
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
      className={({ isActive }) =>
        combineClasses([
          sidebarNavItemStyles,
          isActive ? sidebarNavItemActiveStyles : undefined,
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

const SidebarNavItemIcon: React.FC<{
  icon: ComponentType<{ className?: string }>;
}> = ({ icon: Icon }) => {
  return (
    <Icon
      css={(theme) => ({
        width: theme.spacing(2),
        height: theme.spacing(2),
      })}
    />
  );
};

export const Sidebar: React.FC<{ user: User }> = ({ user }) => {
  const { entitlements } = useDashboard();
  const allowAutostopRequirement =
    entitlements.features.template_autostop_requirement.enabled;

  return (
    <nav css={styles.sidebar}>
      <Stack direction="row" alignItems="center" css={styles.userInfo}>
        <UserAvatar username={user.username} avatarURL={user.avatar_url} />
        <Stack spacing={0} css={styles.userData}>
          <span css={styles.username}>{user.username}</span>
          <span css={styles.email}>{user.email}</span>
        </Stack>
      </Stack>

      <SidebarNavItem
        href="account"
        icon={<SidebarNavItemIcon icon={AccountIcon} />}
      >
        Account
      </SidebarNavItem>
      {allowAutostopRequirement && (
        <SidebarNavItem
          href="schedule"
          icon={<SidebarNavItemIcon icon={ScheduleIcon} />}
        >
          Schedule
        </SidebarNavItem>
      )}
      <SidebarNavItem
        href="security"
        icon={<SidebarNavItemIcon icon={SecurityIcon} />}
      >
        Security
      </SidebarNavItem>
      <SidebarNavItem
        href="ssh-keys"
        icon={<SidebarNavItemIcon icon={FingerprintOutlinedIcon} />}
      >
        SSH Keys
      </SidebarNavItem>
      <SidebarNavItem
        href="tokens"
        icon={<SidebarNavItemIcon icon={VpnKeyOutlined} />}
      >
        Tokens
      </SidebarNavItem>
    </nav>
  );
};

const styles = {
  sidebar: {
    width: 245,
    flexShrink: 0,
  },
  userInfo: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    marginBottom: theme.spacing(2),
  }),
  userData: {
    overflow: "hidden",
  },
  username: {
    fontWeight: 600,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  email: (theme) => ({
    color: theme.palette.text.secondary,
    fontSize: 12,
    overflow: "hidden",
    textOverflow: "ellipsis",
  }),
} satisfies Record<string, Interpolation<Theme>>;
