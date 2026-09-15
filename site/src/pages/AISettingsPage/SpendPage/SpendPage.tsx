import type dayjs from "dayjs";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { paginatedOrganizationAISpend } from "#/api/queries/aiBridge";
import { permittedOrganizations } from "#/api/queries/organizations";
import type {
	AISpendPeriodWindow,
	OrganizationAISpendFilter,
} from "#/api/typesGenerated";
import {
	type DateRangeValue,
	toBoundary,
} from "#/components/DateRangePicker/DateRangePicker";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { useClientFilterMenu } from "#/pages/AIBridgePage/filters/ClientFilter";
import { useModelFilterMenu } from "#/pages/AIBridgePage/filters/ModelFilter";
import { useProviderFilterMenu } from "#/pages/AIBridgePage/filters/ProviderFilter";
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import { selectModelOrganization } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { SpendPageView } from "./SpendPageView";

const organizationSearchParam = "organization";
const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";
const SPEND_USERS_PAGE_SIZE = 10;

type SpendDimensions = Pick<
	OrganizationAISpendFilter,
	"provider_name" | "client" | "model"
>;

// The export authorizes on reading the organization's group members, so the
// picker offers exactly the organizations the export would serve.
const spendOrganizationsCheck = {
	object: { resource_type: "group_member" },
	action: "read",
} as const;

const DAY_MS = 24 * 60 * 60 * 1000;

// Local midnight of the UTC calendar date that the instant falls on.
const localDayOf = (instant: Date): Date =>
	new Date(
		instant.getUTCFullYear(),
		instant.getUTCMonth(),
		instant.getUTCDate(),
	);

/**
 * Represents the server-applied UTC window as the local calendar days the
 * picker edits. A start that is not at UTC midnight is the moving retention
 * cutoff; it advances to the next day so that re-committing the range keeps
 * its start inside retention instead of rounding back before it.
 */
export const appliedWindowToDateRange = (
	window: AISpendPeriodWindow,
	now: Date,
): DateRangeValue => {
	const start = new Date(window.period_start);
	const startsAtMidnight =
		start.getTime() ===
		Date.UTC(start.getUTCFullYear(), start.getUTCMonth(), start.getUTCDate());
	const lastDay = localDayOf(
		new Date(new Date(window.period_end).getTime() - 1),
	);
	let firstDay = localDayOf(start);
	if (!startsAtMidnight && firstDay < lastDay) {
		firstDay = localDayOf(new Date(start.getTime() + DAY_MS));
	}
	return toBoundary(firstDay, lastDay, now);
};

interface SpendPageProps {
	now?: dayjs.Dayjs;
}

const SpendPage: FC<SpendPageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const { isEntitled, isEnabled, hasPermission } = getAIBridgePermissions(
		entitlements,
		permissions,
	);
	const canViewSpend = isEntitled && isEnabled && hasPermission;

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
		...permittedOrganizations(spendOrganizationsCheck),
		enabled: canViewSpend,
	});
	const organizationSelection = selectModelOrganization(
		organizationsQuery.data ?? [],
		searchParams.get(organizationSearchParam),
	);
	const organization = organizationSelection.organization;

	const dimensions: SpendDimensions = {
		provider_name: searchParams.get("provider_name") || undefined,
		client: searchParams.get("client") || undefined,
		model: searchParams.get("model") || undefined,
	};
	const filterMenus = {
		provider: useProviderFilterMenu({
			value: dimensions.provider_name,
			onChange: (option) => setFilterParams({ provider_name: option?.value }),
			enabled: canViewSpend,
		}),
		client: useClientFilterMenu({
			value: dimensions.client,
			onChange: (option) => setFilterParams({ client: option?.value }),
			enabled: canViewSpend,
		}),
		model: useModelFilterMenu({
			value: dimensions.model,
			onChange: (option) => setFilterParams({ model: option?.value }),
			enabled: canViewSpend,
		}),
	};

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
		...dimensions,
	};

	// DateRangePicker already emits exclusive API boundaries (midnight after
	// the picked day, or the next hour when the picked day is today).
	const onDateRangeChange = (value: DateRangeValue) =>
		setFilterParams({
			[startDateSearchParam]: value.startDate.toISOString(),
			[endDateSearchParam]: value.endDate.toISOString(),
		});

	const usersQuery = usePaginatedQuery({
		...paginatedOrganizationAISpend(organization?.id ?? "", spendFilter),
		recordsPerPage: SPEND_USERS_PAGE_SIZE,
		preventScrollReset: true,
		enabled: canViewSpend && organization !== undefined,
	});

	const appliedDateRange =
		dateRange ??
		(usersQuery.data &&
			appliedWindowToDateRange(usersQuery.data, now?.toDate() ?? new Date()));

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("AI Spend")}</title>
			<SpendPageView
				isEntitled={isEntitled}
				isEnabled={isEnabled}
				now={now?.toDate()}
				organizations={organizationsQuery.data ?? []}
				organization={organization}
				onOrganizationChange={(next) =>
					setFilterParams({ [organizationSearchParam]: next.name })
				}
				requestedOrganizationDenied={
					organizationSelection.requestedOrganizationDenied
				}
				isOrganizationsLoading={organizationsQuery.isLoading}
				organizationsError={organizationsQuery.error}
				dateRange={appliedDateRange}
				onDateRangeChange={onDateRangeChange}
				filterMenus={filterMenus}
				usersQuery={usersQuery}
			/>
		</RequirePermission>
	);
};

export default SpendPage;
