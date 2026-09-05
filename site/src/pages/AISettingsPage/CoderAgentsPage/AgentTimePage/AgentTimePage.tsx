import { type FC, useState } from "react";
import { useSearchParams } from "react-router";
import { agentTime } from "#/api/queries/agenttime";
import type { AgentTimeInterval, AgentTimeRequest } from "#/api/typesGenerated";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import AgentTimePageView from "./AgentTimePageView";
import {
	type AgentTimeDatePreset,
	type AgentTimeSortBy,
	type AgentTimeSortOrder,
	type AgentTimeTableGroup,
	activePreset,
	autoInterval,
	dateOnlyToLocalDate,
	inclusiveLocalDateFromExclusiveEnd,
	isAgentTimeInterval,
	isAgentTimeSortBy,
	isAgentTimeSortOrder,
	isAgentTimeTableGroup,
	normalizeDateRange,
	parseDateParam,
	presetRange,
	readSearchParam,
	todayUTC,
	tomorrowUTC,
} from "./agentTimeUtils";

const startDateParam = "start_date";
const endDateParam = "end_date";
const intervalParam = "interval";
const organizationParam = "organization_id";
const userParam = "user_id";
const sortByParam = "sort_by";
const sortOrderParam = "sort_order";

function optionalSearchParam(value: string | null): string | undefined {
	return value === null || value === "" ? undefined : value;
}

interface AgentTimePageProps {
	now?: Date;
}

const AgentTimePage: FC<AgentTimePageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const [initialNow] = useState(() => now ?? new Date());
	const [searchParams, setSearchParams] = useSearchParams();

	const startDate = parseDateParam(searchParams.get(startDateParam));
	const endDate =
		parseDateParam(searchParams.get(endDateParam)) ?? tomorrowUTC(initialNow);
	const interval = readSearchParam<AgentTimeInterval>(
		searchParams,
		intervalParam,
		isAgentTimeInterval,
		autoInterval(startDate, endDate),
	);
	const selectedOrganizationId = optionalSearchParam(
		searchParams.get(organizationParam),
	);
	const selectedUserId = optionalSearchParam(searchParams.get(userParam));
	const sortBy = readSearchParam<AgentTimeSortBy>(
		searchParams,
		sortByParam,
		isAgentTimeSortBy,
		"agent_time",
	);
	const sortOrder = readSearchParam<AgentTimeSortOrder>(
		searchParams,
		sortOrderParam,
		isAgentTimeSortOrder,
		"desc",
	);
	const groupBy = readSearchParam<AgentTimeTableGroup>(
		searchParams,
		"group_by",
		isAgentTimeTableGroup,
		selectedOrganizationId || selectedUserId ? "user" : "organization",
	);

	const request: AgentTimeRequest = {
		...(startDate === undefined ? {} : { start_date: startDate }),
		end_date: endDate,
		interval,
		...(selectedOrganizationId === undefined
			? {}
			: { organization_id: selectedOrganizationId }),
		...(selectedUserId === undefined ? {} : { user_id: selectedUserId }),
		group_by: groupBy,
		sort_by: sortBy,
		sort_order: sortOrder,
	};

	const query = usePaginatedQuery({
		...agentTime(request, {
			organization: selectedOrganizationId,
			searchParams,
		}),
		enabled: permissions.viewDeploymentConfig,
	});

	const resetPageAndSetParams = (update: (next: URLSearchParams) => void) => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			update(next);
			next.delete("page");
			return next;
		});
	};

	const handleDateRangeChange = (value: DateRangeValue) => {
		const range = normalizeDateRange(value);
		resetPageAndSetParams((next) => {
			next.set(startDateParam, range.startDate);
			next.set(endDateParam, range.endDate);
		});
	};

	const handlePresetChange = (preset: AgentTimeDatePreset) => {
		resetPageAndSetParams((next) => {
			if (preset === "all_history") {
				next.delete(startDateParam);
				next.delete(endDateParam);
				next.delete(intervalParam);
				return;
			}
			if (preset === "custom") {
				return;
			}
			const range = presetRange(preset, initialNow);
			next.set(startDateParam, range.startDate);
			next.set(endDateParam, range.endDate);
		});
	};

	const handleIntervalChange = (nextInterval: AgentTimeInterval) => {
		resetPageAndSetParams((next) => {
			next.set(intervalParam, nextInterval);
		});
	};

	const handleSortChange = (nextSortBy: AgentTimeSortBy) => {
		resetPageAndSetParams((next) => {
			const currentSortBy = readSearchParam<AgentTimeSortBy>(
				next,
				sortByParam,
				isAgentTimeSortBy,
				"agent_time",
			);
			const currentSortOrder = readSearchParam<AgentTimeSortOrder>(
				next,
				sortOrderParam,
				isAgentTimeSortOrder,
				"desc",
			);
			next.set(sortByParam, nextSortBy);
			next.set(
				sortOrderParam,
				currentSortBy === nextSortBy && currentSortOrder === "desc"
					? "asc"
					: "desc",
			);
		});
	};

	const handleSelectOrganization = (organizationId: string) => {
		resetPageAndSetParams((next) => {
			next.set(organizationParam, organizationId);
			next.set("group_by", "user");
			next.delete(userParam);
			next.set(sortByParam, "agent_time");
			next.set(sortOrderParam, "desc");
		});
	};

	const handleClearOrganization = () => {
		resetPageAndSetParams((next) => {
			next.delete(organizationParam);
			next.set("group_by", "organization");
			next.delete(userParam);
			next.set(sortByParam, "agent_time");
			next.set(sortOrderParam, "desc");
		});
	};

	const handleSelectUser = (userId: string) => {
		resetPageAndSetParams((next) => {
			next.set(userParam, userId);
		});
	};

	const handleClearUser = () => {
		resetPageAndSetParams((next) => {
			next.delete(userParam);
		});
	};

	const fallbackStartDate =
		startDate ?? query.data?.start_date ?? todayUTC(initialNow);
	const dateRange: DateRangeValue = {
		startDate: dateOnlyToLocalDate(fallbackStartDate),
		endDate: inclusiveLocalDateFromExclusiveEnd(endDate),
	};

	return (
		<RequirePermission isFeatureVisible={permissions.viewDeploymentConfig}>
			<AgentTimePageView
				query={query}
				now={initialNow}
				dateRange={dateRange}
				activePreset={activePreset(startDate, endDate, initialNow)}
				isAllHistory={startDate === undefined}
				endDate={endDate}
				interval={interval}
				sortBy={sortBy}
				sortOrder={sortOrder}
				tableGroup={groupBy}
				onGroupChange={(nextGroup) => {
					resetPageAndSetParams((next) => {
						next.set("group_by", nextGroup);
						next.delete(userParam);
					});
				}}
				selectedOrganizationId={selectedOrganizationId}
				selectedUserId={selectedUserId}
				onDateRangeChange={handleDateRangeChange}
				onPresetChange={handlePresetChange}
				onIntervalChange={handleIntervalChange}
				onSortChange={handleSortChange}
				onSelectOrganization={handleSelectOrganization}
				onClearOrganization={handleClearOrganization}
				onSelectUser={handleSelectUser}
				onClearUser={handleClearUser}
				onRetry={() => {
					void query.refetch();
				}}
			/>
		</RequirePermission>
	);
};

export default AgentTimePage;
