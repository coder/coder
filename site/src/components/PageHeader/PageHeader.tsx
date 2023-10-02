import { type FC, type PropsWithChildren, type ReactNode } from "react";
import { Stack } from "../Stack/Stack";

export interface PageHeaderProps {
  actions?: ReactNode;
  className?: string;
}

export const PageHeader: FC<PropsWithChildren<PageHeaderProps>> = ({
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
        paddingTop: theme.spacing(6),
        paddingBottom: theme.spacing(6),
        gap: theme.spacing(4),

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
              marginTop: theme.spacing(3),
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

export const PageHeaderTitle: FC<PropsWithChildren<unknown>> = ({
  children,
}) => {
  return (
    <h1
      css={(theme) => ({
        fontSize: theme.spacing(3),
        fontWeight: 400,
        margin: 0,
        display: "flex",
        alignItems: "center",
        lineHeight: "140%",
      })}
    >
      {children}
    </h1>
  );
};

export const PageHeaderSubtitle: FC<
  PropsWithChildren<{ condensed?: boolean }>
> = ({ children, condensed }) => {
  return (
    <h2
      css={(theme) => ({
        fontSize: theme.spacing(2),
        color: theme.palette.text.secondary,
        fontWeight: 400,
        display: "block",
        margin: 0,
        marginTop: condensed ? theme.spacing(0.5) : theme.spacing(1),
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
