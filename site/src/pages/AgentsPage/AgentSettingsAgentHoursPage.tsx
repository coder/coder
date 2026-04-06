import dayjs from "dayjs";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { chatRuntimeSummary } from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsAgentHoursPageView } from "./AgentSettingsAgentHoursPageView";

const createDateRange = (now?: dayjs.Dayjs) => {
	const end = now ?? dayjs();
	const start = end.subtract(30, "day");
	return {
		startDate: start.toISOString(),
		endDate: end.toISOString(),
		rangeLabel: `${start.format("MMM D")} – ${end.format("MMM D, YYYY")}`,
	};
};

interface AgentSettingsAgentHoursPageProps {
	/** Override the current time for deterministic storybook snapshots. */
	now?: dayjs.Dayjs;
}

const AgentSettingsAgentHoursPage: FC<AgentSettingsAgentHoursPageProps> = ({
	now,
}) => {
	const { permissions } = useAuthenticated();
	const [anchor] = useState<dayjs.Dayjs>(() => dayjs());
	const dateRange = createDateRange(now ?? anchor);

	const runtimeQuery = useQuery({
		...chatRuntimeSummary({
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
	});

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsAgentHoursPageView
				data={runtimeQuery.data}
				isLoading={runtimeQuery.isLoading}
				error={runtimeQuery.error}
				onRetry={() => void runtimeQuery.refetch()}
				rangeLabel={dateRange.rangeLabel}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsAgentHoursPage;
