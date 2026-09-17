import { saveAs } from "file-saver";
import { type FC, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { useSearchParams } from "react-router";
import {
	exportOrganizationAISpend,
	paginatedOrganizationAISpendDetails,
} from "#/api/queries/aiBridge";
import { permittedOrganizations } from "#/api/queries/organizations";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import { selectModelOrganization } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { useSpendDetailsFilterMenus } from "./components/SpendDetailsFilters";
import { SpendDetailsPageView } from "./SpendDetailsPageView";
import {
	appliedWindowToDateRange,
	dateRangeFromSearchParams,
	displayDateRange,
	retentionMinDate,
	spendDetailsFilter,
	spendEndDateSearchParam,
	spendOrganizationCheck,
	spendOrganizationSearchParam,
	spendStartDateSearchParam,
} from "./spendDetails";

const SPEND_DETAILS_PAGE_SIZE = 25;

interface SpendDetailsPageProps {
	now?: Date;
}

const SpendDetailsPage: FC<SpendDetailsPageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const { isEntitled, isEnabled, hasPermission } = getAIBridgePermissions(
		entitlements,
		permissions,
	);
	const canViewSpend = isEntitled && isEnabled && hasPermission;
	const [searchParams, setSearchParams] = useSearchParams();
	const [currentTime] = useState(() => now ?? new Date());

	const updateSearchParams = (updates: Record<string, string | undefined>) => {
		setSearchParams(
			(previous) => {
				const next = new URLSearchParams(previous);
				for (const [key, value] of Object.entries(updates)) {
					if (value) next.set(key, value);
					else next.delete(key);
				}
				next.delete("page");
				return next;
			},
			{ replace: true },
		);
	};

	const organizationsQuery = useQuery({
		...permittedOrganizations(spendOrganizationCheck),
		enabled: canViewSpend,
	});
	const organizationSelection = selectModelOrganization(
		organizationsQuery.data ?? [],
		searchParams.get(spendOrganizationSearchParam),
	);
	const organization = organizationSelection.requestedOrganizationDenied
		? undefined
		: organizationSelection.organization;
	const filter = spendDetailsFilter(searchParams);
	const query = usePaginatedQuery({
		...paginatedOrganizationAISpendDetails(organization?.id ?? "", filter),
		recordsPerPage: SPEND_DETAILS_PAGE_SIZE,
		preventScrollReset: true,
		enabled: canViewSpend && organization !== undefined,
	});
	const minDate = retentionMinDate(query.data?.retention_start, currentTime);
	const explicitDateRange = dateRangeFromSearchParams(searchParams);
	const dateRange =
		(explicitDateRange && displayDateRange(explicitDateRange)) ??
		(query.data
			? appliedWindowToDateRange(query.data, currentTime)
			: undefined);
	const menus = useSpendDetailsFilterMenus({
		organization: organization?.id ?? "",
		values: filter,
		onChange: (key, value) => updateSearchParams({ [key]: value }),
		enabled: canViewSpend && organization !== undefined,
	});
	const hasExplicitDateRange = filter.period_start !== undefined;
	const appliedExportFilter = {
		...filter,
		...(filter.period_start === undefined &&
			query.data &&
			query.data.period_start !== query.data.retention_start && {
				period_start: query.data.period_start,
				period_end: query.data.period_end,
			}),
	};
	const exportMutation = useMutation({
		...exportOrganizationAISpend(),
		onSuccess: (file) => saveAs(file, "ai-spend-details.csv"),
	});
	const canExport =
		organization !== undefined &&
		!query.isPlaceholderData &&
		(hasExplicitDateRange || query.data !== undefined) &&
		(hasExplicitDateRange || query.error === null);

	const onDateRangeChange = (value: DateRangeValue) =>
		updateSearchParams({
			[spendStartDateSearchParam]: value.startDate.toISOString(),
			[spendEndDateSearchParam]: value.endDate.toISOString(),
		});

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("AI Spend Details")}</title>
			<SpendDetailsPageView
				isEntitled={isEntitled}
				isEnabled={isEnabled}
				organization={organization}
				isOrganizationsLoading={organizationsQuery.isLoading}
				requestedOrganizationDenied={
					organizationSelection.requestedOrganizationDenied
				}
				organizationsError={organizationsQuery.error}
				filters={{
					organizations: organizationsQuery.data ?? [],
					onOrganizationChange: (next) =>
						updateSearchParams({
							[spendOrganizationSearchParam]: next.name,
							user_id: undefined,
							group_id: undefined,
						}),
					dateRange,
					now: currentTime,
					minDate,
					onDateRangeChange,
					menus,
				}}
				onExport={() =>
					exportMutation.mutate({
						organizationId: organization?.id ?? "",
						filter: appliedExportFilter,
					})
				}
				isExporting={exportMutation.isPending}
				canExport={canExport}
				exportError={exportMutation.error}
				query={query}
			/>
		</RequirePermission>
	);
};

export default SpendDetailsPage;
