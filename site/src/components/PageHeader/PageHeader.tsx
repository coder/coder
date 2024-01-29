import { type FC, type PropsWithChildren, type ReactNode } from "react";
import { Stack } from "../Stack/Stack";

export interface PageHeaderProps {
  actions?: ReactNode;
  className?: string;
  children?: ReactNode;
}

export const PageHeader: FC<PageHeaderProps> = ({
  children,
  actions,
  className,
}) => {
  return (
    <header
      className={className}
      css={(theme) => ({
        display: "flex",
        alignItems: "center",
        paddingTop: 48,
        paddingBottom: 48,
        gap: 32,

        [theme.breakpoints.down("md")]: {
          flexDirection: "column",
          alignItems: "flex-start",
        },
      })}
      data-testid="header"
    >
      <hgroup>{children}</hgroup>
      {actions && (
        <Stack
          direction="row"
          css={(theme) => ({
            marginLeft: "auto",

            [theme.breakpoints.down("md")]: {
              marginLeft: "initial",
              width: "100%",
            },
          })}
        >
          {actions}
        </Stack>
      )}
    </header>
  );
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
  return (
    <h1
      css={{
        fontSize: 24,
        fontWeight: 400,
        margin: 0,
        display: "flex",
        alignItems: "center",
        lineHeight: "140%",
      }}
    >
      {children}
    </h1>
  );
};

interface PageHeaderSubtitleProps {
  children?: ReactNode;
  condensed?: boolean;
}

export const PageHeaderSubtitle: FC<PageHeaderSubtitleProps> = ({
  children,
  condensed,
}) => {
  return (
    <h2
      css={(theme) => ({
        fontSize: 16,
        color: theme.palette.text.secondary,
        fontWeight: 400,
        display: "block",
        margin: 0,
        marginTop: condensed ? 4 : 8,
        lineHeight: "140%",
      })}
    >
      {children}
    </h2>
  );
};

export const PageHeaderCaption: FC<PropsWithChildren> = ({ children }) => {
  return (
    <span
      css={(theme) => ({
        fontSize: 12,
        color: theme.palette.text.secondary,
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: "0.1em",
      })}
    >
      {children}
    </span>
  );
};
