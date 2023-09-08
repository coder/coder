import { makeStyles, useTheme } from "@mui/styles";
import { FC } from "react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { colors } from "theme/colors";
import { Stack } from "components/Stack/Stack";
import { Theme } from "@mui/material/styles";

dayjs.extend(relativeTime);

type CircleProps = { color: string; variant?: "solid" | "outlined" };

const Circle: FC<CircleProps> = ({ color, variant = "solid" }) => {
  const styles = useCircleStyles({ color, variant });
  return <div className={styles.root} />;
};

const useCircleStyles = makeStyles((theme) => ({
  root: {
    width: theme.spacing(1),
    height: theme.spacing(1),
    backgroundColor: (props: CircleProps) =>
      props.variant === "solid" ? props.color : undefined,
    border: (props: CircleProps) =>
      props.variant === "outlined" ? `1px solid ${props.color}` : undefined,
    borderRadius: 9999,
  },
}));

interface LastUsedProps {
  lastUsedAt: string;
}

export const LastUsed: FC<LastUsedProps> = ({ lastUsedAt }) => {
  const theme: Theme = useTheme();
  const styles = useStyles();
  const t = dayjs(lastUsedAt);
  const now = dayjs();
  let message = t.fromNow();
  let circle: JSX.Element = (
    <Circle color={theme.palette.text.secondary} variant="outlined" />
  );

  if (t.isAfter(now.subtract(1, "hour"))) {
    circle = <Circle color={colors.green[9]} />;
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now";
  } else if (t.isAfter(now.subtract(3, "day"))) {
    circle = <Circle color={theme.palette.text.secondary} />;
  } else if (t.isAfter(now.subtract(1, "month"))) {
    circle = <Circle color={theme.palette.warning.light} />;
  } else if (t.isAfter(now.subtract(100, "year"))) {
    circle = <Circle color={colors.red[10]} />;
  } else {
    // color = theme.palette.error.light
    message = "Never";
  }

  return (
    <Stack
      className={styles.root}
      direction="row"
      spacing={1}
      alignItems="center"
    >
      {circle}
      <span data-chromatic="ignore">{message}</span>
    </Stack>
  );
};

const useStyles = makeStyles((theme) => ({
  root: {
    color: theme.palette.text.secondary,
  },
}));
