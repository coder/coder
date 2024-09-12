import { useTheme } from "@emotion/react";
import { Stack } from "components/Stack/Stack";
import { StatusIndicator } from "components/StatusIndicator/StatusIndicator";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { useTime } from "hooks/useTime";
import type { FC } from "react";

dayjs.extend(relativeTime);
interface LastUsedProps {
	lastUsedAt: string;
}

export const LastUsed: FC<LastUsedProps> = ({ lastUsedAt }) => {
	const theme = useTheme();

	const [circle, message] = useTime(() => {
		const t = dayjs(lastUsedAt);
		const now = dayjs();
		let message = t.fromNow();
		let circle = <StatusIndicator color="info" variant="outlined" />;

		if (t.isAfter(now.subtract(1, "hour"))) {
			circle = <StatusIndicator color="success" />;
			// Since the agent reports on a 10m interval,
			// the last_used_at can be inaccurate when recent.
			message = "Now";
		} else if (t.isAfter(now.subtract(3, "day"))) {
			circle = <StatusIndicator color="info" />;
		} else if (t.isAfter(now.subtract(1, "month"))) {
			circle = <StatusIndicator color="warning" />;
		} else if (t.isAfter(now.subtract(100, "year"))) {
			circle = <StatusIndicator color="error" />;
		} else {
			message = "Never";
		}

		return [circle, message];
	});

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
