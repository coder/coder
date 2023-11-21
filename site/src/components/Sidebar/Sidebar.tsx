import { cx } from "@emotion/css";
import { type CSSObject, type Interpolation, type Theme } from "@emotion/react";
import { type ElementType, type FC, type ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import { Stack } from "components/Stack/Stack";
import { type ClassName, useClassName } from "hooks/useClassName";

interface SidebarProps {
  children?: ReactNode;
}

export const Sidebar: FC<SidebarProps> = ({ children }) => {
  return <nav css={styles.sidebar}>{children}</nav>;
};

interface SidebarHeaderProps {
  avatar: ReactNode;
  title: ReactNode;
  subtitle: ReactNode;
  linkTo?: string;
}

export const SidebarHeader: FC<SidebarHeaderProps> = ({
  avatar,
  title,
  subtitle,
  linkTo,
}) => {
  return (
    <Stack direction="row" alignItems="center" css={styles.info}>
      {avatar}
      <div css={styles.data}>
        {linkTo ? (
          <Link css={styles.title} to={linkTo}>
            {title}
          </Link>
        ) : (
          <span css={styles.title}>{title}</span>
        )}
        <span css={styles.subtitle}>{subtitle}</span>
      </div>
    </Stack>
  );
};

interface SidebarNavItemProps {
  children?: ReactNode;
  icon: ElementType;
  href: string;
}

export const SidebarNavItem: FC<SidebarNavItemProps> = ({
  children,
  href,
  icon: Icon,
}) => {
  const link = useClassName(classNames.link, []);
  const activeLink = useClassName(classNames.activeLink, []);

  return (
    <NavLink
      end
      to={href}
      className={({ isActive }) => cx([link, isActive && activeLink])}
    >
      <Stack alignItems="center" spacing={1.5} direction="row">
        <Icon css={{ width: 16, height: 16 }} />
        {children}
      </Stack>
    </NavLink>
  );
};

const styles = {
  sidebar: {
    width: 245,
    flexShrink: 0,
  },
  info: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    marginBottom: 16,
  }),
  data: {
    overflow: "hidden",
  },
  title: (theme) => ({
    fontWeight: 600,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    color: theme.palette.text.primary,
    textDecoration: "none",
  }),
  subtitle: (theme) => ({
    color: theme.palette.text.secondary,
    fontSize: 12,
    overflow: "hidden",
    textOverflow: "ellipsis",
  }),
} satisfies Record<string, Interpolation<Theme>>;

const classNames = {
  link: (css, theme) => css`
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
  `,

  activeLink: (css, theme) => css`
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
  `,
} satisfies Record<string, ClassName>;
