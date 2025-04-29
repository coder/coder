import { Stack } from "components/Stack/Stack";
import { StatusIndicatorDot } from "components/StatusIndicator/StatusIndicator";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { IDEAL_REFRESH_ONE_MINUTE, useTimeSync } from "hooks/useTimeSync";
import type { FC } from "react";

dayjs.extend(relativeTime);
interface LastUsedProps {
	lastUsedAt: string;
}

export const LastUsed: FC<LastUsedProps> = ({ lastUsedAt }) => {
	/**
	 * @todo Verify that this is equivalent
	 */
	const [circle, message] = useTimeSync({
		idealRefreshIntervalMs: IDEAL_REFRESH_ONE_MINUTE,
		select: (date) => {
			const t = dayjs(lastUsedAt);
			const deltaMsg = t.from(dayjs(date));
			const circle = <StatusIndicatorDot variant="inactive" />;
			return [circle, deltaMsg] as const;
		},
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
