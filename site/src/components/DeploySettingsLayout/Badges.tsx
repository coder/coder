import { makeStyles } from "@mui/styles";
import { Stack } from "components/Stack/Stack";
import { PropsWithChildren, FC } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { combineClasses } from "utils/combineClasses";
import Tooltip from "@mui/material/Tooltip";

export const EnabledBadge: FC = () => {
  const styles = useStyles();
  return (
    <span className={combineClasses([styles.badge, styles.enabledBadge])}>
      Enabled
    </span>
  );
};

export const EntitledBadge: FC = () => {
  const styles = useStyles();
  return (
    <span className={combineClasses([styles.badge, styles.enabledBadge])}>
      Entitled
    </span>
  );
};

export const HealthyBadge: FC<{ derpOnly: boolean }> = ({ derpOnly }) => {
  const styles = useStyles();
  let text = "Healthy";
  if (derpOnly) {
    text = "Healthy (DERP Only)";
  }
  return (
    <span className={combineClasses([styles.badge, styles.enabledBadge])}>
      {text}
    </span>
  );
};

export const NotHealthyBadge: FC = () => {
  const styles = useStyles();
  return (
    <span className={combineClasses([styles.badge, styles.errorBadge])}>
      Unhealthy
    </span>
  );
};

export const NotRegisteredBadge: FC = () => {
  const styles = useStyles();
  return (
    <Tooltip title="Workspace Proxy has never come online and needs to be started.">
      <span className={combineClasses([styles.badge, styles.warnBadge])}>
        Never Seen
      </span>
    </Tooltip>
  );
};

export const NotReachableBadge: FC = () => {
  const styles = useStyles();
  return (
    <Tooltip title="Workspace Proxy not responding to http(s) requests.">
      <span className={combineClasses([styles.badge, styles.warnBadge])}>
        Not Dialable
      </span>
    </Tooltip>
  );
};

export const DisabledBadge: FC = () => {
  const styles = useStyles();
  return (
    <span className={combineClasses([styles.badge, styles.disabledBadge])}>
      Disabled
    </span>
  );
};

export const EnterpriseBadge: FC = () => {
  const styles = useStyles();
  return (
    <span className={combineClasses([styles.badge, styles.enterpriseBadge])}>
      Enterprise
    </span>
  );
};

export const Badges: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles();
  return (
    <Stack
      className={styles.badges}
      direction="row"
      alignItems="center"
      spacing={1}
    >
      {children}
    </Stack>
  );
};

const useStyles = makeStyles((theme) => ({
  badges: {
    margin: theme.spacing(0, 0, 2),
  },

  badge: {
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
  },

  enterpriseBadge: {
    backgroundColor: theme.palette.info.dark,
    border: `1px solid ${theme.palette.info.light}`,
  },

  versionBadge: {
    border: `1px solid ${theme.palette.success.light}`,
    backgroundColor: theme.palette.success.dark,
    textTransform: "none",
    color: "white",
    fontFamily: MONOSPACE_FONT_FAMILY,
    textDecoration: "none",
    fontSize: 12,
  },

  enabledBadge: {
    border: `1px solid ${theme.palette.success.light}`,
    backgroundColor: theme.palette.success.dark,
  },

  errorBadge: {
    border: `1px solid ${theme.palette.error.light}`,
    backgroundColor: theme.palette.error.dark,
  },

  warnBadge: {
    border: `1px solid ${theme.palette.warning.light}`,
    backgroundColor: theme.palette.warning.dark,
  },

  disabledBadge: {
    border: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
  },
}));
