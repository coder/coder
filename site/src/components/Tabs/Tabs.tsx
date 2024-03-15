import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { createContext, type FC, type HTMLAttributes, useContext } from "react";
import { Link, type LinkProps } from "react-router-dom";

export const TAB_PADDING_Y = 12;
export const TAB_PADDING_X = 16;

type TabsContextValue = {
  active: string;
};

const TabsContext = createContext<TabsContextValue | undefined>(undefined);

type TabsProps = HTMLAttributes<HTMLDivElement> & TabsContextValue;

export const Tabs: FC<TabsProps> = ({ active, ...htmlProps }) => {
  const theme = useTheme();

  return (
    <TabsContext.Provider value={{ active }}>
      <div
        css={{
          borderBottom: `1px solid ${theme.palette.divider}`,
        }}
        {...htmlProps}
      />
    </TabsContext.Provider>
  );
};

type TabsListProps = HTMLAttributes<HTMLDivElement>;

export const TabsList: FC<TabsListProps> = (props) => {
  return (
    <div
      role="tablist"
      css={{
        display: "flex",
        alignItems: "baseline",
      }}
      {...props}
    />
  );
};

type TabLinkProps = LinkProps & {
  value: string;
};

export const TabLink: FC<TabLinkProps> = ({ value, ...linkProps }) => {
  const tabsContext = useContext(TabsContext);

  if (!tabsContext) {
    throw new Error("Tab only can be used inside of Tabs");
  }

  const isActive = tabsContext.active === value;

  return (
    <Link
      {...linkProps}
      css={[styles.tabLink, isActive ? styles.activeTabLink : ""]}
    />
  );
};

const styles = {
  tabLink: (theme) => ({
    textDecoration: "none",
    color: theme.palette.text.secondary,
    fontSize: 14,
    display: "block",
    padding: `${TAB_PADDING_Y}px ${TAB_PADDING_X}px`,
    fontWeight: 500,
    lineHeight: "1",

    "&:hover": {
      color: theme.palette.text.primary,
    },
  }),
  activeTabLink: (theme) => ({
    color: theme.palette.text.primary,
    position: "relative",

    "&:before": {
      content: '""',
      left: 0,
      bottom: -1,
      height: 1,
      width: "100%",
      background: theme.palette.primary.main,
      position: "absolute",
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
