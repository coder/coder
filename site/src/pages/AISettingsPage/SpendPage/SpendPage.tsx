import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import {
	aiSpendOrganizations,
	paginatedOrganizationAISpend,
} from "#/api/queries/aiBridge";
import type {
	OrganizationAISpendFilter,
	OrganizationAISpendReport,
} from "#/api/typesGenerated";
import {
	type DateRangeValue,
	toBoundary,
} from "#/components/DateRangePicker/DateRangePicker";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
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

type SpendDimensions = Pick<
	OrganizationAISpendFilter,
	"provider_name" | "client" | "model"
>;

// Local midnight of the UTC calendar date that the instant falls on.
const localDayOf = (instant: Date): Date =>
	new Date(
		instant.getUTCFullYear(),
		instant.getUTCMonth(),
		instant.getUTCDate(),
	);

/**
 * The first local calendar day the picker can select on or after the moving
 * retention cutoff: the first local midnight at or after it. With less than a
 * day of retention that midnight has not happened yet.
 */
export const firstDayWithinRetention = (cutoff: Date): Date => {
	const day = new Date(
		cutoff.getFullYear(),
		cutoff.getMonth(),
		cutoff.getDate(),
	);
	return day < cutoff
		? new Date(cutoff.getFullYear(), cutoff.getMonth(), cutoff.getDate() + 1)
		: day;
};

/**
 * Maps the server's UTC window to local picker days, advancing the start past
 * retention where possible. Undefined when no selectable day remains, since
 * every day the picker could commit would start before the cutoff.
 */
export const appliedWindowToDateRange = (
	reportWindow: Pick<
		OrganizationAISpendReport,
		"period_start" | "period_end" | "retention_start"
	>,
	now: Date,
): DateRangeValue | undefined => {
	const start = new Date(reportWindow.period_start);
	let lastDay = localDayOf(
		new Date(new Date(reportWindow.period_end).getTime() - 1),
	);
	let firstDay = localDayOf(start);
	if (
		reportWindow.retention_start !== undefined &&
		firstDay.getTime() < Date.parse(reportWindow.retention_start)
	) {
		firstDay = firstDayWithinRetention(new Date(reportWindow.retention_start));
		const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
		if (firstDay > today) {
			return undefined;
		}
		if (firstDay > lastDay) {
			lastDay = firstDay;
		}
	}
	return toBoundary(firstDay, lastDay, now);
};

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
	// The provider, model, and client options come from deployment-wide AI
	// Gateway endpoints, so only viewers of every session get those filters.
	const canFilterDimensions = permissions.viewAnyAIBridgeInterception;

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
		searchParams.get(organizationSearchParam),
	);
	// A requested organization the viewer cannot see gets a warning, not
	// another organization's spend.
	const organization = organizationSelection.requestedOrganizationDenied
		? undefined
		: organizationSelection.organization;

	const dimensions: SpendDimensions = canFilterDimensions
		? {
				provider_name: searchParams.get("provider_name") || undefined,
				client: searchParams.get("client") || undefined,
				model: searchParams.get("model") || undefined,
			}
		: {};
	const filterMenus = {
		provider: useProviderFilterMenu({
			value: dimensions.provider_name,
			onChange: (option) => setFilterParams({ provider_name: option?.value }),
			enabled: isSpendAvailable && canFilterDimensions,
		}),
		client: useClientFilterMenu({
			value: dimensions.client,
			onChange: (option) => setFilterParams({ client: option?.value }),
			enabled: isSpendAvailable && canFilterDimensions,
		}),
		model: useModelFilterMenu({
			value: dimensions.model,
			onChange: (option) => setFilterParams({ model: option?.value }),
			enabled: isSpendAvailable && canFilterDimensions,
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

	const reportQuery = usePaginatedQuery({
		...paginatedOrganizationAISpend(organization?.id ?? "", spendFilter),
		recordsPerPage: 10,
		preventScrollReset: true,
		enabled: isSpendAvailable && organization !== undefined,
	});

	const currentTime = now ?? new Date();
	const appliedDateRange =
		dateRange ??
		(reportQuery.data &&
			appliedWindowToDateRange(reportQuery.data, currentTime));
	// Usage before the retention cutoff has been purged, so the picker does not
	// offer those days. Absent when the deployment does not purge.
	const retentionStart = reportQuery.data?.retention_start;
	const minDate = retentionStart
		? firstDayWithinRetention(new Date(retentionStart))
		: undefined;

	return (
		<>
			<title>{pageTitle("Spend", "AI Settings")}</title>
			<SpendPageView
				isEntitled={isEntitled}
				isEnabled={isEnabled}
				now={now}
				organizations={organizationsQuery.data ?? []}
				organization={organization}
				onOrganizationChange={(next) =>
					setFilterParams({ [organizationSearchParam]: next.name })
				}
				isOrganizationsLoading={organizationsQuery.isLoading}
				organizationsError={organizationsQuery.error}
				dateRange={appliedDateRange}
				minDate={minDate}
				onDateRangeChange={onDateRangeChange}
				filterMenus={canFilterDimensions ? filterMenus : undefined}
				reportQuery={reportQuery}
			/>
		</>
	);
};

export default SpendPage;
