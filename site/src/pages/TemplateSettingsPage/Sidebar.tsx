import { css } from "@emotion/css";
import {
  useTheme,
  type CSSObject,
  type Interpolation,
  type Theme,
} from "@emotion/react";
import ScheduleIcon from "@mui/icons-material/TimerOutlined";
import VariablesIcon from "@mui/icons-material/CodeOutlined";
import type { Template } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import {
  type FC,
  type ElementType,
  type PropsWithChildren,
  type ReactNode,
} from "react";
import { Link, NavLink } from "react-router-dom";
import GeneralIcon from "@mui/icons-material/SettingsOutlined";
import SecurityIcon from "@mui/icons-material/LockOutlined";
import { Avatar } from "components/Avatar/Avatar";
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
    padding: 12px 12px 12px 16px;
    border-radius: 4px;
    transition: background-color 0.15s ease-in-out;
    margin-bottom: 1px;
    position: relative;

    &:hover {
      background-color: ${theme.palette.action.hover};
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
      background-color: ${theme.palette.primary.main};
      border-top-left-radius: 8px;
      border-bottom-left-radius: 8px;
    }
  `;

  return (
    <NavLink
      end
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

const SidebarNavItemIcon: React.FC<{ icon: ElementType }> = ({
  icon: Icon,
}) => {
  return <Icon css={styles.sidebarNavItemIcon} />;
};

export const Sidebar: React.FC<{ template: Template }> = ({ template }) => {
  return (
    <nav css={styles.sidebar}>
      <Stack direction="row" alignItems="center" css={styles.templateInfo}>
        <Avatar src={template.icon} variant="square" fitImage />
        <Stack spacing={0} css={styles.templateData}>
          <Link css={styles.name} to={`/templates/${template.name}`}>
            {template.display_name !== ""
              ? template.display_name
              : template.name}
          </Link>
          <span css={styles.secondary}>{template.name}</span>
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

const styles = {
  sidebar: {
    width: 245,
    flexShrink: 0,
  },
  sidebarNavItemIcon: {
    width: 16,
    height: 16,
  },
  templateInfo: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    marginBottom: 16,
  }),
  templateData: {
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
