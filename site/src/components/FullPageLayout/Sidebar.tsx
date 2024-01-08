import { Interpolation, Theme, useTheme } from "@mui/material/styles";
import { ComponentProps, HTMLAttributes } from "react";
import { Link, LinkProps } from "react-router-dom";
import { TopbarIconButton } from "./Topbar";

export const Sidebar = (props: HTMLAttributes<HTMLDivElement>) => {
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

export const SidebarLink = (props: LinkProps) => {
  return <Link css={styles.sidebarItem} {...props} />;
};

export const SidebarItem = (
  props: HTMLAttributes<HTMLButtonElement> & { isActive?: boolean },
) => {
  const { isActive, ...buttonProps } = props;
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

export const SidebarCaption = (props: HTMLAttributes<HTMLSpanElement>) => {
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

export const SidebarIconButton = (
  props: { isActive: boolean } & ComponentProps<typeof TopbarIconButton>,
) => {
  const theme = useTheme();

  return (
    <TopbarIconButton
      css={[
        { opacity: 0.75, "&:hover": { opacity: 1 } },
        props.isActive && {
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
        },
      ]}
      {...props}
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
} satisfies Record<string, Interpolation<Theme>>;
