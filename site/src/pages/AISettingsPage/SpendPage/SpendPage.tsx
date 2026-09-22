import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import {
	aiSpendOrganizations,
	paginatedOrganizationAISpend,
} from "#/api/queries/aiBridge";
import type { OrganizationAISpendFilter } from "#/api/typesGenerated";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { SpendPageView } from "./SpendPageView";

const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";

type SpendPageProps = {
	now?: Date;
};

const SpendPage: FC<SpendPageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const { isEntitled, isEnabled } = getAIBridgePermissions(
		entitlements,
		permissions,
	);
	const isSpendAvailable = isEntitled && isEnabled;

	const [searchParams, setSearchParams] = useSearchParams();

	const setFilterParams = (updates: Record<string, string | undefined>) => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				for (const [key, value] of Object.entries(updates)) {
					if (value) {
						next.set(key, value);
					} else {
						next.delete(key);
					}
				}
				next.delete("page");
				return next;
			},
			{ replace: true },
		);
	};

	const organizationsQuery = useQuery({
		...aiSpendOrganizations(),
		enabled: isSpendAvailable,
	});
	const organizationSelection = selectModelOrganization(
		organizationsQuery.data ?? [],
		searchParams.get(modelOrganizationSearchParam),
	);
	// A requested organization the viewer cannot see gets a warning, not
	// another organization's spend.
	const organization = organizationSelection.requestedOrganizationDenied
		? undefined
		: organizationSelection.organization;

	const startDateParam = searchParams.get(startDateSearchParam)?.trim() ?? "";
	const endDateParam = searchParams.get(endDateSearchParam)?.trim() ?? "";

	// Without an explicit range the server applies the current budget period
	// narrowed to retention, which a client-side default could not know.
	let dateRange: DateRangeValue | undefined;

	if (startDateParam && endDateParam) {
		const parsedStartDate = new Date(startDateParam);
		const parsedEndDate = new Date(endDateParam);

		if (
			!Number.isNaN(parsedStartDate.getTime()) &&
			!Number.isNaN(parsedEndDate.getTime()) &&
			parsedStartDate.getTime() < parsedEndDate.getTime()
		) {
			dateRange = {
				startDate: parsedStartDate,
				endDate: parsedEndDate,
			};
		}
	}

	const spendFilter: OrganizationAISpendFilter = {
		...(dateRange && {
			period_start: dateRange.startDate.toISOString(),
			period_end: dateRange.endDate.toISOString(),
		}),
	};

	// DateRangePicker already emits exclusive API boundaries (midnight after
	// the picked day, or the next hour when the picked day is today).
	const onDateRangeChange = (value: DateRangeValue) =>
		setFilterParams({
			[startDateSearchParam]: value.startDate.toISOString(),
			[endDateSearchParam]: value.endDate.toISOString(),
		});

	const reportQuery = usePaginatedQuery({
		...paginatedOrganizationAISpend(organization?.id ?? "", spendFilter),
		recordsPerPage: 10,
		preventScrollReset: true,
		enabled: isSpendAvailable && organization !== undefined,
	});

	return (
		<>
			<title>{pageTitle("User spend", "AI Settings")}</title>
			<SpendPageView
				isEntitled={isEntitled}
				isEnabled={isEnabled}
				now={now}
				organizations={organizationsQuery.data ?? []}
				organization={organization}
				onOrganizationChange={(next) =>
					setFilterParams({ [modelOrganizationSearchParam]: next.name })
				}
				isOrganizationsLoading={organizationsQuery.isLoading}
				organizationsError={organizationsQuery.error}
				dateRange={dateRange}
				minDate={undefined}
				onDateRangeChange={onDateRangeChange}
				filterMenus={undefined}
				reportQuery={reportQuery}
			/>
		</>
	);
};

export default SpendPage;
