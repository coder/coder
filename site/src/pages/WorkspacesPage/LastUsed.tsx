import { type FC } from "react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { useTheme } from "@emotion/react";
import { Stack } from "components/Stack/Stack";

dayjs.extend(relativeTime);

type CircleProps = {
  color: string;
  variant?: "solid" | "outlined";
};

const Circle: FC<CircleProps> = ({ color, variant = "solid" }) => {
  return (
    <div
      aria-hidden
      css={{
        width: 8,
        height: 8,
        backgroundColor: variant === "solid" ? color : undefined,
        border: variant === "outlined" ? `1px solid ${color}` : undefined,
        borderRadius: 9999,
      }}
    />
  );
};

interface LastUsedProps {
  lastUsedAt: string;
}

export const LastUsed: FC<LastUsedProps> = ({ lastUsedAt }) => {
  const theme = useTheme();
  const t = dayjs(lastUsedAt);
  const now = dayjs();
  let message = t.fromNow();
  let circle = (
    <Circle color={theme.palette.text.secondary} variant="outlined" />
  );

  if (t.isAfter(now.subtract(1, "hour"))) {
    circle = <Circle color={theme.experimental.roles.success.fill} />;
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now";
  } else if (t.isAfter(now.subtract(3, "day"))) {
    circle = <Circle color={theme.palette.text.secondary} />;
  } else if (t.isAfter(now.subtract(1, "month"))) {
    circle = <Circle color={theme.palette.warning.light} />;
  } else if (t.isAfter(now.subtract(100, "year"))) {
    circle = <Circle color={theme.experimental.roles.error.fill} />;
  } else {
    message = "Never";
  }

  return (
    <Stack
      style={{ color: theme.palette.text.secondary }}
      direction="row"
      spacing={1}
      alignItems="center"
    >
      {circle}
      <span data-chromatic="ignore">{message}</span>
    </Stack>
  );
};
