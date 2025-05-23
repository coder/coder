import { useTheme } from "@emotion/react";
import { isAfter, sub } from "date-fns";
import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";
import { type DateValue, relativeTime, toDate } from "utils/time";

interface LastSeenProps
	extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
	at: DateValue | undefined;
}

export const LastSeen: FC<LastSeenProps> = ({ at, className, ...attrs }) => {
	const theme = useTheme();
	let message = "Never";
	let color = theme.palette.text.secondary;

	if (at) {
		const t = toDate(at);
		const now = new Date();
		message = relativeTime(t);

		if (isAfter(t, sub(now, { hours: 1 }))) {
			message = "Now";
			color = theme.roles.success.fill.solid;
		} else if (isAfter(t, sub(now, { days: 3 }))) {
			color = theme.experimental.l2.text;
		} else if (isAfter(t, sub(now, { months: 1 }))) {
			color = theme.roles.warning.fill.solid;
		} else if (isAfter(t, sub(now, { years: 100 }))) {
			color = theme.roles.error.fill.solid;
		}
	}

	return (
		<span
			{...attrs}
			css={{ color }}
			className={cn([
				"whitespace-nowrap first-letter:uppercase block",
				className,
			])}
		>
			{message}
		</span>
	);
};
