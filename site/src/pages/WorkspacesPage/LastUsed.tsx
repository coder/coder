import { Stack } from "components/Stack/Stack";
import { StatusIndicatorDot } from "components/StatusIndicator/StatusIndicator";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { useTime } from "hooks/useTime";
import type { FC } from "react";

dayjs.extend(relativeTime);
interface LastUsedProps {
	lastUsedAt: string;
}

export const LastUsed: FC<LastUsedProps> = ({ lastUsedAt }) => {
	const [circle, message] = useTime(() => {
		const t = dayjs(lastUsedAt);
		const now = dayjs();
		let message = t.fromNow();
		let circle = <StatusIndicatorDot variant="inactive" />;

		if (t.isAfter(now.subtract(1, "hour"))) {
			circle = <StatusIndicatorDot variant="success" />;
			// Since the agent reports on a 10m interval,
			// the last_used_at can be inaccurate when recent.
			message = "Now";
		} else if (t.isAfter(now.subtract(3, "day"))) {
			circle = <StatusIndicatorDot variant="pending" />;
		} else if (t.isAfter(now.subtract(1, "month"))) {
			circle = <StatusIndicatorDot variant="warning" />;
		} else if (t.isAfter(now.subtract(100, "year"))) {
			circle = <StatusIndicatorDot variant="failed" />;
		} else {
			message = "Never";
		}

		return [circle, message];
	});

	return (
		<Stack
			className="text-content-secondary"
			direction="row"
			spacing={1}
			alignItems="center"
		>
			{circle}
			<span data-chromatic="ignore">{message}</span>
		</Stack>
	);
};