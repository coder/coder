import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import {
	agentRuntimeInsights,
	agentRuntimeInsightsByUserPaginated,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import {
	CoderAgentsUsagePageView,
	DEFAULT_RANGE,
	rangeFromSearchParams,
} from "./CoderAgentsUsagePageView";

const CoderAgentsUsagePage: FC = () => {
	const { permissions } = useAuthenticated();
	const [searchParams, setSearchParams] = useSearchParams();
	const range = rangeFromSearchParams(searchParams) ?? DEFAULT_RANGE;
	const [sort, setSort] = useState<{
		column: "username" | "totalMs" | "messageCount";
		direction: "asc" | "desc";
	}>({ column: "totalMs", direction: "desc" });

	const summaryQuery = useQuery(agentRuntimeInsights(range));
	const usersQuery = usePaginatedQuery(
		agentRuntimeInsightsByUserPaginated(range, searchParams),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("Coder Agents Usage", "AI Settings")}</title>

			<CoderAgentsUsagePageView
				range={range}
				onRangeChange={(newRange) => {
					searchParams.set("startDate", newRange.startTime);
					searchParams.set("endDate", newRange.endTime);
					usersQuery.goToFirstPage();
					setSearchParams(searchParams);
				}}
				summaryData={summaryQuery.data}
				summaryError={summaryQuery.error}
				isLoadingSummary={summaryQuery.isLoading}
				usersQuery={usersQuery}
				sortColumn={sort.column}
				sortDirection={sort.direction}
				onSortChange={(column, direction) => setSort({ column, direction })}
			/>
		</RequirePermission>
	);
};

export default CoderAgentsUsagePage;
