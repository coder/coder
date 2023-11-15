import { ReactNode } from "react";
import { NavLink, NavLinkProps } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";
import { Margins } from "components/Margins/Margins";
import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";

export const Tabs = ({ children }: { children: ReactNode }) => {
  return (
    <div
      css={(theme) => ({
        borderBottom: `1px solid ${theme.palette.divider}`,
        marginBottom: 40,
      })}
    >
      <Margins
        css={{
          display: "flex",
          alignItems: "center",
          gap: 2,
        }}
      >
        {children}
      </Margins>
    </div>
  );
};

export const TabLink = (props: NavLinkProps) => {
  const theme = useTheme();

  const baseTabLink = css`
    text-decoration: none;
    color: ${theme.palette.text.secondary};
    font-size: 14px;
    display: block;
    padding: 0 16px 16px;

    &:hover {
      color: ${theme.palette.text.primary};
    }
  `;

  const activeTabLink = css`
    color: ${theme.palette.text.primary};
    position: relative;

    &:before {
      content: "";
      left: 0;
      bottom: 0;
      height: 2px;
      width: 100%;
      background: ${theme.palette.secondary.dark};
      position: absolute;
    }
  `;

  return (
    <NavLink
      className={({ isActive }) =>
        combineClasses([
          baseTabLink,
          isActive ? activeTabLink : undefined,
          props.className as string,
        ])
      }
      {...props}
    />
  );
};
