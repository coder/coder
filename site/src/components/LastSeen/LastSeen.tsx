import { useTheme } from "@emotion/react";
import { isAfter, sub } from "date-fns";
import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";
import { relativeTime, toDate } from "utils/time";

interface LastSeenProps
	extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
	at: import("utils/time").DateValue;
	"data-chromatic"?: string; // prevents a type error in the stories
}

export const LastSeen: FC<LastSeenProps> = ({ at, className, ...attrs }) => {
	const theme = useTheme();
	const t = toDate(at);
	const now = new Date();

	let message = relativeTime(t);
	let color = theme.palette.text.secondary;

	if (isAfter(t, sub(now, { hours: 1 }))) {
		// Since the agent reports on a 10m interval,
		// the last_used_at can be inaccurate when recent.
		message = "Now";
		color = theme.roles.success.fill.solid;
	} else if (isAfter(t, sub(now, { days: 3 }))) {
		color = theme.experimental.l2.text;
	} else if (isAfter(t, sub(now, { months: 1 }))) {
		color = theme.roles.warning.fill.solid;
	} else if (isAfter(t, sub(now, { years: 100 }))) {
		color = theme.roles.error.fill.solid;
	} else {
		message = "Never";
	}

	return (
		<span
			data-chromatic="ignore"
			css={{ color }}
			{...attrs}
			className={cn(["whitespace-nowrap", className])}
		>
			{message}
		</span>
	);
};
