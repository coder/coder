import { cn } from "cn";
import dayjs from "dayjs";
import type { ComponentProps, FC } from "react";
import { isAfter, subtractTime, timeFrom } from "#/utils/time";

type LastSeenProps = Omit<ComponentProps<"span">, "children"> & {
	at: dayjs.ConfigType;
	isExactDays?: boolean;
	// Injectable reference time so the component is deterministic in tests.
	now?: dayjs.ConfigType;
	"data-pixel"?: string; // prevents a type error in the stories
};

const displayFor = (
	at: dayjs.ConfigType,
	now: dayjs.ConfigType,
	isExactDays: boolean,
): { message: string; color: string } => {
	const relativeMessage = isExactDays
		? exactDaysFrom(at, now)
		: timeFrom(at, now);

	if (isAfter(at, subtractTime(now, 1, "hour"))) {
		// Since the agent reports on a 10m interval,
		// the last_used_at can be inaccurate when recent.
		return { message: "Now", color: "text-content-success" };
	}
	if (isAfter(at, subtractTime(now, 3, "day"))) {
		return { message: relativeMessage, color: "text-content-primary" };
	}
	if (isAfter(at, subtractTime(now, 1, "month"))) {
		return { message: relativeMessage, color: "text-content-warning" };
	}
	if (isAfter(at, subtractTime(now, 100, "year"))) {
		return { message: relativeMessage, color: "text-content-destructive" };
	}
	return { message: "Never", color: "text-content-secondary" };
};

const exactDaysFrom = (at: dayjs.ConfigType, now: dayjs.ConfigType) => {
	const days = dayjs(now).startOf("day").diff(dayjs(at).startOf("day"), "day");
	if (days <= 0) {
		return "Today";
	}
	return days === 1 ? "1 day ago" : `${days} days ago`;
};

export const LastSeen: FC<LastSeenProps> = ({
	at,
	isExactDays = false,
	now = new Date(),
	className,
	...attrs
}) => {
	const { message, color } = displayFor(at, now, isExactDays);

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
