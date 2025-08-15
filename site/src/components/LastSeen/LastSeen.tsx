import { useTheme } from "@emotion/react";
import dayjs from "dayjs";
import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";
import { isAfter, relativeTime, subtractTime } from "utils/time";

interface LastSeenProps
	extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
	at: dayjs.ConfigType;
	"data-chromatic"?: string; // prevents a type error in the stories
}

export const LastSeen: FC<LastSeenProps> = ({ at, className, ...attrs }) => {
	const theme = useTheme();
	const _t = dayjs(at);
	const now = new Date();
	const oneHourAgo = subtractTime(now, 1, "hour");
	const threeDaysAgo = subtractTime(now, 3, "day");
	const oneMonthAgo = subtractTime(now, 1, "month");
	const centuryAgo = subtractTime(now, 100, "year");

	let message = relativeTime(at);
	let color = theme.palette.text.secondary;

	if (isAfter(at, oneHourAgo)) {
		// Since the agent reports on a 10m interval,
		// the last_used_at can be inaccurate when recent.
		message = "Now";
		color = theme.roles.success.fill.solid;
	} else if (isAfter(at, threeDaysAgo)) {
		color = theme.experimental.l2.text;
	} else if (isAfter(at, oneMonthAgo)) {
		color = theme.roles.warning.fill.solid;
	} else if (isAfter(at, centuryAgo)) {
		color = theme.roles.error.fill.solid;
	} else {
		message = "Never";
	}

	return (
		<span
			data-chromatic="ignore"
			style={{ color }}
			{...attrs}
			className={cn(["whitespace-nowrap", className])}
		>
			{message}
		</span>
	);
};
