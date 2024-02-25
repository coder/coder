import { cx } from "@emotion/css";
import { type FC, type PropsWithChildren } from "react";
import { NavLink, NavLinkProps } from "react-router-dom";
import { Margins } from "components/Margins/Margins";
import { makeClassNames } from "hooks/useClassNames";

export const Tabs: FC<PropsWithChildren> = ({ children }) => {
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

interface TabLinkProps extends NavLinkProps {
  className?: string;
}

export const TabLink: FC<TabLinkProps> = ({
  className,
  children,
  ...linkProps
}) => {
  const classNames = useClassNames(null);

  return (
    <NavLink
      className={({ isActive }) =>
        cx([
          classNames.tabLink,
          isActive && classNames.activeTabLink,
          className,
        ])
      }
      {...linkProps}
    >
      {children}
    </NavLink>
  );
};

const useClassNames = makeClassNames((css, theme) => ({
  tabLink: css`
    text-decoration: none;
    color: ${theme.palette.text.secondary};
    font-size: 14px;
    display: block;
    padding: 0 16px 16px;

    &:hover {
      color: ${theme.palette.text.primary};
    }
  `,
  activeTabLink: css`
    color: ${theme.palette.text.primary};
    position: relative;

    &:before {
      content: "";
      left: 0;
      bottom: 0;
      height: 2px;
      width: 100%;
      background: ${theme.palette.primary.main};
      position: absolute;
    }
  `,
}));
