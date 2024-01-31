import { useTheme } from "@emotion/react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { type FC, type HTMLAttributes } from "react";

dayjs.extend(relativeTime);

interface LastSeenProps
  extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
  at: dayjs.ConfigType;
  "data-chromatic"?: string; // prevents a type error in the stories
}

export const LastSeen: FC<LastSeenProps> = ({ at, ...attrs }) => {
  const theme = useTheme();
  const t = dayjs(at);
  const now = dayjs();

  let message = t.fromNow();
  let color = theme.palette.text.secondary;

  if (t.isAfter(now.subtract(1, "hour"))) {
    // Since the agent reports on a 10m interval,
    // the last_used_at can be inaccurate when recent.
    message = "Now";
    color = theme.experimental.roles.success.fill.solid;
  } else if (t.isAfter(now.subtract(3, "day"))) {
    color = theme.experimental.l2.text;
  } else if (t.isAfter(now.subtract(1, "month"))) {
    color = theme.experimental.roles.warning.fill.solid;
  } else if (t.isAfter(now.subtract(100, "year"))) {
    color = theme.experimental.roles.error.fill.solid;
  } else {
    message = "Never";
  }

  return (
    <span data-chromatic="ignore" css={{ color }} {...attrs}>
      {message}
    </span>
  );
};
