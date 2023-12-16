import { useTheme } from "@emotion/react";
import dayjs from "dayjs";
import { type FC, type HTMLAttributes } from "react";

interface LastSeenProps extends HTMLAttributes<HTMLSpanElement> {
  value: string;
}

export const LastSeen: FC<LastSeenProps> = ({ value, ...attrs }) => {
  const theme = useTheme();
  const t = dayjs(value);
  const now = dayjs();

  let message = t.fromNow();
  let color = theme.palette.text.secondary;

  if (t.isAfter(now.subtract(1, "hour"))) {
    color = theme.palette.success.light;
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now";
  } else if (t.isAfter(now.subtract(3, "day"))) {
    color = theme.palette.text.secondary;
  } else if (t.isAfter(now.subtract(1, "month"))) {
    color = theme.palette.warning.light;
  } else if (t.isAfter(now.subtract(100, "year"))) {
    color = theme.palette.error.light;
  } else {
    message = "Never";
  }

  return (
    <span data-chromatic="ignore" css={{ color }} {...attrs}>
      {message}
    </span>
  );
};
