import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import type { ComponentProps, FC, HTMLAttributes } from "react";
import { Link, type LinkProps } from "react-router-dom";
import { TopbarIconButton } from "./Topbar";

export const Sidebar: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
  const theme = useTheme();
  return (
    <div
      css={{
        width: 260,
        borderRight: `1px solid ${theme.palette.divider}`,
        height: "100%",
        overflow: "auto",
        flexShrink: 0,
        padding: "8px 0",
        display: "flex",
        flexDirection: "column",
        gap: 1,
      }}
      {...props}
    />
  );
};

export const SidebarLink: FC<LinkProps> = (props) => {
  return <Link css={styles.sidebarItem} {...props} />;
};

interface SidebarItemProps extends HTMLAttributes<HTMLButtonElement> {
  isActive?: boolean;
}

export const SidebarItem: FC<SidebarItemProps> = ({
  isActive,
  ...buttonProps
}) => {
  const theme = useTheme();

  return (
    <button
      css={[
        styles.sidebarItem,
        { opacity: "0.75", "&:hover": { opacity: 1 } },
        isActive && {
          background: theme.palette.action.selected,
          opacity: 1,
          pointerEvents: "none",
        },
      ]}
      {...buttonProps}
    />
  );
};

export const SidebarCaption: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
  return (
    <span
      css={{
        fontSize: 10,
        lineHeight: 1.2,
        padding: "12px 16px",
        display: "block",
        textTransform: "uppercase",
        fontWeight: 500,
        letterSpacing: "0.1em",
      }}
      {...props}
    />
  );
};

interface SidebarIconButton extends ComponentProps<typeof TopbarIconButton> {
  isActive: boolean;
}

export const SidebarIconButton: FC<SidebarIconButton> = ({
  isActive,
  ...buttonProps
}) => {
  return (
    <TopbarIconButton
      css={[
        { opacity: 0.75, "&:hover": { opacity: 1 } },
        isActive && styles.activeSidebarIconButton,
      ]}
      {...buttonProps}
    />
  );
};

const styles = {
  sidebarItem: (theme) => ({
    fontSize: 13,
    lineHeight: 1.2,
    color: theme.palette.text.primary,
    textDecoration: "none",
    padding: "8px 16px",
    display: "block",
    textAlign: "left",
    background: "none",
    border: 0,
    cursor: "pointer",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  }),

  activeSidebarIconButton: (theme) => ({
    opacity: 1,
    position: "relative",
    "&::before": {
      content: '""',
      position: "absolute",
      left: 0,
      top: 0,
      bottom: 0,
      width: 2,
      backgroundColor: theme.palette.primary.main,
      height: "100%",
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
