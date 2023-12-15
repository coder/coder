import type { PropsWithChildren, FC } from "react";
import Tooltip from "@mui/material/Tooltip";
import { type Interpolation, type Theme } from "@emotion/react";
import { Stack } from "components/Stack/Stack";
import colors from "theme/tailwind";

const styles = {
  badge: {
    fontSize: 10,
    height: 24,
    fontWeight: 600,
    textTransform: "uppercase",
    letterSpacing: "0.085em",
    padding: "0 12px",
    borderRadius: 9999,
    display: "flex",
    alignItems: "center",
    width: "fit-content",
    whiteSpace: "nowrap",
  },

  enabledBadge: (theme) => ({
    border: `1px solid ${theme.experimental.roles.success.outline}`,
    backgroundColor: theme.experimental.roles.success.background,
  }),
  errorBadge: (theme) => ({
    border: `1px solid ${theme.experimental.roles.error.outline}`,
    backgroundColor: theme.experimental.roles.error.background,
  }),
  warnBadge: (theme) => ({
    border: `1px solid ${theme.experimental.roles.warning.outline}`,
    backgroundColor: theme.experimental.roles.warning.background,
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
        {
          border: `1px solid ${colors.violet[600]}`,
          backgroundColor: colors.violet[950],
          color: colors.violet[50],
        },
      ]}
    >
      Alpha
    </span>
  );
};

export const DeprecatedBadge: FC = () => {
  return (
    <span
      css={[
        styles.badge,
        {
          border: `1px solid ${colors.orange[600]}`,
          backgroundColor: colors.orange[950],
          color: colors.orange[50],
        },
      ]}
    >
      Deprecated
    </span>
  );
};

export const Badges: FC<PropsWithChildren> = ({ children }) => {
  return (
    <Stack
      css={{ margin: "0 0 16px" }}
      direction="row"
      alignItems="center"
      spacing={1}
    >
      {children}
    </Stack>
  );
};
