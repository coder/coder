import type { PropsWithChildren, FC } from "react";
import Tooltip from "@mui/material/Tooltip";
import { type Interpolation, type Theme } from "@emotion/react";
import { Stack } from "components/Stack/Stack";

const styles = {
  badge: (theme) => ({
    fontSize: 10,
    height: 24,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: "0.085em",
    padding: theme.spacing(0, 1.5),
    borderRadius: 9999,
    display: "flex",
    alignItems: "center",
    width: "fit-content",
    whiteSpace: "nowrap",
  }),

  enabledBadge: (theme) => ({
    border: `1px solid ${theme.palette.success.light}`,
    backgroundColor: theme.palette.success.dark,
  }),
  errorBadge: (theme) => ({
    border: `1px solid ${theme.palette.error.light}`,
    backgroundColor: theme.palette.error.dark,
  }),
  warnBadge: (theme) => ({
    border: `1px solid ${theme.palette.warning.light}`,
    backgroundColor: theme.palette.warning.dark,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export const EnabledBadge: FC = () => {
  return <span css={[styles.badge, styles.enabledBadge]}>Enabled</span>;
};

export const EntitledBadge: FC = () => {
  return <span css={[styles.badge, styles.enabledBadge]}>Entitled</span>;
};

interface HealthyBadge {
  derpOnly: boolean;
}
export const HealthyBadge: FC<HealthyBadge> = (props) => {
  const { derpOnly } = props;
  return (
    <span css={[styles.badge, styles.enabledBadge]}>
      {derpOnly ? "Healthy (DERP only)" : "Healthy"}
    </span>
  );
};

export const NotHealthyBadge: FC = () => {
  return <span css={[styles.badge, styles.errorBadge]}>Unhealthy</span>;
};

export const NotRegisteredBadge: FC = () => {
  return (
    <Tooltip title="Workspace Proxy has never come online and needs to be started.">
      <span css={[styles.badge, styles.warnBadge]}>Never seen</span>
    </Tooltip>
  );
};

export const NotReachableBadge: FC = () => {
  return (
    <Tooltip title="Workspace Proxy not responding to http(s) requests.">
      <span css={[styles.badge, styles.warnBadge]}>Not reachable</span>
    </Tooltip>
  );
};

export const DisabledBadge: FC = () => {
  return (
    <span
      css={[
        styles.badge,
        (theme) => ({
          border: `1px solid ${theme.palette.divider}`,
          backgroundColor: theme.palette.background.paper,
        }),
      ]}
    >
      Disabled
    </span>
  );
};

export const EnterpriseBadge: FC = () => {
  return (
    <span
      css={[
        styles.badge,
        (theme) => ({
          backgroundColor: theme.palette.info.dark,
          border: `1px solid ${theme.palette.info.light}`,
        }),
      ]}
    >
      Enterprise
    </span>
  );
};

export const AlphaBadge: FC = () => {
  return (
    <span
      css={[
        styles.badge,
        (theme) => ({
          border: `1px solid ${theme.palette.error.light}`,
          backgroundColor: theme.palette.error.dark,
        }),
      ]}
    >
      Alpha
    </span>
  );
};

export const Badges: FC<PropsWithChildren> = ({ children }) => {
  return (
    <Stack
      css={(theme) => ({
        margin: theme.spacing(0, 0, 2),
      })}
      direction="row"
      alignItems="center"
      spacing={1}
    >
      {children}
    </Stack>
  );
};
