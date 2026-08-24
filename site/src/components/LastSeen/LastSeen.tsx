import type dayjs from "dayjs";
import type { FC, HTMLAttributes } from "react";
import { cn } from "#/utils/cn";
import { isAfter, subtractTime, timeFrom } from "#/utils/time";

interface LastSeenProps
	extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
	at: dayjs.ConfigType;
	// Injectable reference time so the component is deterministic in tests.
	now?: dayjs.ConfigType;
	"data-pixel"?: string; // prevents a type error in the stories
}

const displayFor = (
	at: dayjs.ConfigType,
	now: dayjs.ConfigType,
): { message: string; color: string } => {
	if (isAfter(at, subtractTime(now, 1, "hour"))) {
		// Since the agent reports on a 10m interval,
		// the last_used_at can be inaccurate when recent.
		return { message: "Now", color: "text-content-success" };
	}
	if (isAfter(at, subtractTime(now, 3, "day"))) {
		return { message: timeFrom(at, now), color: "text-content-primary" };
	}
	if (isAfter(at, subtractTime(now, 1, "month"))) {
		return { message: timeFrom(at, now), color: "text-content-warning" };
	}
	if (isAfter(at, subtractTime(now, 100, "year"))) {
		return { message: timeFrom(at, now), color: "text-content-destructive" };
	}
	return { message: "Never", color: "text-content-secondary" };
};

export const LastSeen: FC<LastSeenProps> = ({
	at,
	now = new Date(),
	className,
	...attrs
}) => {
	const { message, color } = displayFor(at, now);

	return (
		<span
			data-pixel="ignore"
			{...attrs}
			className={cn(["whitespace-nowrap", color, className])}
		>
			{message}
		</span>
	);
};
