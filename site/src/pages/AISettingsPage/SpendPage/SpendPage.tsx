import { type FC, useState } from "react";
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
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { SpendPageView } from "./SpendPageView";

const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";

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
 * retention where possible. Undefined when no selectable day remains: the
 * first local day is still ahead of the viewer's clock, either because the
 * UTC period began after local midnight or because every day the picker could
 * commit would start before the retention cutoff.
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
		if (firstDay > lastDay) {
			lastDay = firstDay;
		}
	}
	const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
	if (firstDay > today) {
		return undefined;
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

	const currentTime = now ?? new Date();
	const appliedDateRange =
		dateRange ??
		(reportQuery.data &&
			appliedWindowToDateRange(reportQuery.data, currentTime));

	// Retention is deployment-wide. Keep the last reported cutoff while the
	// next report loads so the date picker stays mounted.
	const [retention, setRetention] = useState<{ start: string | undefined }>();
	if (
		reportQuery.data &&
		!reportQuery.isPlaceholderData &&
		(retention === undefined ||
			retention.start !== reportQuery.data.retention_start)
	) {
		setRetention({ start: reportQuery.data.retention_start });
	}
	const minDate = retention?.start
		? firstDayWithinRetention(new Date(retention.start))
		: undefined;

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
				dateRange={appliedDateRange}
				minDate={minDate}
				isRetentionLoading={retention === undefined && reportQuery.isLoading}
				onDateRangeChange={onDateRangeChange}
				filterMenus={undefined}
				reportQuery={reportQuery}
			/>
		</>
	);
};

export default SpendPage;
