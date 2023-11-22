import { type FC, type HTMLAttributes } from "react";
import { colors } from "theme/colors";

export const Sidebar: FC<HTMLAttributes<HTMLElement>> = ({
  children,
  ...attrs
}) => {
  return (
    <nav
      css={(theme) => ({
        width: 256,
        flexShrink: 0,
        borderRight: `1px solid ${theme.palette.divider}`,
        height: "100%",
        overflowY: "auto",
      })}
      {...attrs}
    >
      {children}
    </nav>
  );
};

interface SidebarItemProps extends HTMLAttributes<HTMLElement> {
  active?: boolean;
}

export const SidebarItem: FC<SidebarItemProps> = ({
  children,
  active,
  ...attrs
}) => {
  return (
    <button
      css={(theme) => ({
        background: active ? colors.gray[13] : "none",
        border: "none",
        fontSize: 14,
        width: "100%",
        textAlign: "left",
        padding: "0 24px",
        cursor: "pointer",
        pointerEvents: active ? "none" : "auto",
        color: active
          ? theme.palette.text.primary
          : theme.palette.text.secondary,
        "&:hover": {
          background: theme.palette.action.hover,
          color: theme.palette.text.primary,
        },
        paddingTop: 10,
        paddingBottom: 10,
      })}
      {...attrs}
    >
      {children}
    </button>
  );
};

export const SidebarCaption: FC<HTMLAttributes<HTMLDivElement>> = ({
  children,
  ...attrs
}) => {
  return (
    <div
      css={(theme) => ({
        fontSize: 10,
        textTransform: "uppercase",
        fontWeight: 500,
        color: theme.palette.text.secondary,
        padding: "12px 24px",
        letterSpacing: "0.5px",
      })}
      {...attrs}
    >
      {children}
    </div>
  );
};
