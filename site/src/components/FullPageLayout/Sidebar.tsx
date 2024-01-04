import { Interpolation, Theme, useTheme } from "@mui/material/styles";
import { HTMLAttributes } from "react";
import { Link, LinkProps } from "react-router-dom";

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

export const SidebarItem = (props: HTMLAttributes<HTMLButtonElement>) => {
  return <button css={styles.sidebarItem} {...props} />;
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
        letterSpacing: 1,
      }}
      {...props}
    />
  );
};

const styles = {
  sidebarItem: (theme: Theme) => ({
    fontSize: 13,
    lineHeight: 1.2,
    color: theme.palette.text.primary,
    textDecoration: "none",
    padding: "8px 16px",
    display: "block",
    textAlign: "left",
    background: "none",
    border: 0,

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
