import { type CSSObject, useTheme } from "@emotion/react";
import type { FC, PropsWithChildren, ReactNode } from "react";

interface FullWidthPageHeaderProps {
  children?: ReactNode;
  sticky?: boolean;
}

export const FullWidthPageHeader: FC<FullWidthPageHeaderProps> = ({
  children,
  sticky = true,
}) => {
  const theme = useTheme();
  return (
    <header
      data-testid="header"
      css={[
        {
          ...(theme.typography.body2 as CSSObject),
          padding: 24,
          background: theme.palette.background.default,
          borderBottom: `1px solid ${theme.palette.divider}`,
          display: "flex",
          alignItems: "center",
          gap: 48,

          zIndex: 10,
          flexWrap: "wrap",

          [theme.breakpoints.down("lg")]: {
            position: "unset",
            alignItems: "flex-start",
          },
          [theme.breakpoints.down("md")]: {
            flexDirection: "column",
          },
        },
        sticky && {
          position: "sticky",
          top: 0,
        },
      ]}
    >
      {children}
    </header>
  );
};

export const PageHeaderActions: FC<PropsWithChildren> = ({ children }) => {
  const theme = useTheme();
  return (
    <div
      css={{
        marginLeft: "auto",
        [theme.breakpoints.down("md")]: {
          marginLeft: "unset",
        },
      }}
    >
      {children}
    </div>
  );
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
  return (
    <h1
      css={{
        fontSize: 18,
        fontWeight: 500,
        margin: 0,
        lineHeight: "24px",
      }}
    >
      {children}
    </h1>
  );
};

export const PageHeaderSubtitle: FC<PropsWithChildren> = ({ children }) => {
  const theme = useTheme();
  return (
    <span
      css={{
        fontSize: 14,
        color: theme.palette.text.secondary,
        display: "block",
      }}
    >
      {children}
    </span>
  );
};
