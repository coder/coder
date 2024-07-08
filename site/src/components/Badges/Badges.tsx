import type { Interpolation, Theme } from "@emotion/react";
import Tooltip from "@mui/material/Tooltip";
import {
  type FC,
  forwardRef,
  type HTMLAttributes,
  type PropsWithChildren,
} from "react";
import { Stack } from "components/Stack/Stack";

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
    border: `1px solid ${theme.roles.success.outline}`,
    backgroundColor: theme.roles.success.background,
    color: theme.roles.success.text,
  }),
  errorBadge: (theme) => ({
    border: `1px solid ${theme.roles.error.outline}`,
    backgroundColor: theme.roles.error.background,
    color: theme.roles.error.text,
  }),
  warnBadge: (theme) => ({
    border: `1px solid ${theme.roles.warning.outline}`,
    backgroundColor: theme.roles.warning.background,
    color: theme.roles.warning.text,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export const EnabledBadge: FC = () => {
  return (
    <span css={[styles.badge, styles.enabledBadge]} className="option-enabled">
      Enabled
    </span>
  );
};

export const EntitledBadge: FC = () => {
  return <span css={[styles.badge, styles.enabledBadge]}>Entitled</span>;
};

interface HealthyBadge {
  derpOnly?: boolean;
}
export const HealthyBadge: FC<HealthyBadge> = ({ derpOnly }) => {
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

export const DisabledBadge: FC = forwardRef<
  HTMLSpanElement,
  HTMLAttributes<HTMLSpanElement>
>((props, ref) => {
  return (
    <span
      {...props}
      ref={ref}
      css={[
        styles.badge,
        (theme) => ({
          border: `1px solid ${theme.experimental.l1.outline}`,
          backgroundColor: theme.experimental.l1.background,
          color: theme.experimental.l1.text,
        }),
      ]}
      className="option-disabled"
    >
      Disabled
    </span>
  );
});

export const EnterpriseBadge: FC = () => {
  return (
    <span
      css={[
        styles.badge,
        (theme) => ({
          backgroundColor: theme.roles.info.background,
          border: `1px solid ${theme.roles.info.outline}`,
          color: theme.roles.info.text,
        }),
      ]}
    >
      Enterprise
    </span>
  );
};

export const PreviewBadge: FC = () => {
  return (
    <span
      css={[
        styles.badge,
        (theme) => ({
          border: `1px solid ${theme.roles.preview.outline}`,
          backgroundColor: theme.roles.preview.background,
          color: theme.roles.preview.text,
        }),
      ]}
    >
      Preview
    </span>
  );
};

export const AlphaBadge: FC = () => {
  return (
    <span
      css={[
        styles.badge,
        (theme) => ({
          border: `1px solid ${theme.roles.preview.outline}`,
          backgroundColor: theme.roles.preview.background,
          color: theme.roles.preview.text,
        }),
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
        (theme) => ({
          border: `1px solid ${theme.roles.danger.outline}`,
          backgroundColor: theme.roles.danger.background,
          color: theme.roles.danger.text,
        }),
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
