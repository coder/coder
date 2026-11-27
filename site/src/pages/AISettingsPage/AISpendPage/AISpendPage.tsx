import { saveAs } from "file-saver";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { getErrorDetail } from "#/api/errors";
import {
	aiGatewayRetention,
	aiSpendOrganizations,
	exportOrganizationAISpend,
	organizationAISpendOverview,
	organizationAISpendReportsScopeKey,
	organizationAISpendUsers,
	organizationUnpricedAIModels,
	paginatedOrganizationAISpendReport,
} from "#/api/queries/aiBridge";
import { useFilter, useFilterParamsKey } from "#/components/Filter/Filter";
import { toast } from "#/components/Toaster/toast";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useUpdateSearchParams } from "#/hooks/useUpdateSearchParams";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { AISpendPageView, type AISpendTab } from "./AISpendPageView";
import { toReportFilter } from "./DetailedReportFilter";
import {
	defaultSpendDays,
	firstRetainedDay,
	parseSpendDays,
	type SpendDays,
	toSpendPeriod,
} from "./spendDays";
import { TOP_SPENDERS_COUNT } from "./topSpenders";
import { UNPRICED_MODELS_COUNT } from "./unpricedModels";

const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";
const tabSearchParam = "tab";
// The number of users each Detailed report page lists.
const DETAILED_REPORT_PAGE_SIZE = 10;

type ReportRun = {
	organizationId: string;
	days: SpendDays;
	filter: string;
};

type AISpendPageProps = {
	now?: Date;
};

const AISpendPage: React.FC<AISpendPageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const { entitlements, organizations } = useDashboard();
	const { isEntitled, isEnabled } = getAIBridgePermissions(
		entitlements,
		permissions,
	);
	const isSpendAvailable = isEntitled && isEnabled;
	const queryClient = useQueryClient();

	const [searchParams, setSearchParams] = useSearchParams();
	const updateSearchParams = useUpdateSearchParams();
	const setSearchParamValues = (values: Record<string, string | undefined>) => {
		updateSearchParams((params) => {
			for (const [key, value] of Object.entries(values)) {
				if (value === undefined) {
					params.delete(key);
				} else {
					params.set(key, value);
				}
			}
		});
	};

	const organizationsQuery = useQuery({
		...aiSpendOrganizations(organizations),
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

	// Only some views read data that gets purged, but the tabs share the days,
	// so every view starts no earlier than the retention.
	const retentionQuery = useQuery({
		...aiGatewayRetention(),
		enabled: isSpendAvailable,
	});
	const minDay =
		retentionQuery.data === undefined
			? undefined
			: firstRetainedDay(now ?? new Date(), retentionQuery.data.retention_ms);
	// The spend waits for the retention, so it doesn't request purged days.
	const isSpendReady =
		isSpendAvailable && organization !== undefined && !retentionQuery.isPending;

	// The default range stays out of the URL, so a shared link without dates
	// resolves against the viewer's last 7 days.
	const days =
		parseSpendDays(
			searchParams.get(startDateSearchParam),
			searchParams.get(endDateSearchParam),
			minDay,
		) ?? defaultSpendDays(now ?? new Date(), minDay);
	const period = toSpendPeriod(days);
	const reportFilter = useFilter({
		searchParams,
		onSearchParamsChange: (next) => setSearchParams(next, { replace: true }),
	});
	// Viewers who can't use the filter ignore one from a shared link.
	const appliedReportFilter = permissions.viewAnyAIBridgeInterception
		? reportFilter.query
		: "";
	const [reportRun, setReportRun] = useState<ReportRun>();
	const lastReportRun =
		reportRun?.organizationId === organization?.id ? reportRun : undefined;
	const isReportOutdated =
		lastReportRun !== undefined &&
		(lastReportRun.days.startDate !== days.startDate ||
			lastReportRun.days.endDate !== days.endDate ||
			lastReportRun.filter !== appliedReportFilter);
	const reportQuery = usePaginatedQuery({
		...paginatedOrganizationAISpendReport(
			queryClient,
			lastReportRun?.organizationId ?? "",
			toSpendPeriod(lastReportRun?.days ?? days),
			toReportFilter(lastReportRun?.filter ?? ""),
		),
		recordsPerPage: DETAILED_REPORT_PAGE_SIZE,
		preventScrollReset: true,
		enabled: lastReportRun !== undefined,
	});
	// The page whose rows are shown, so a canceled page change can return to it.
	const [shownReportPage, setShownReportPage] = useState(1);
	if (
		reportQuery.data !== undefined &&
		!reportQuery.isPlaceholderData &&
		reportQuery.currentPage !== shownReportPage
	) {
		setShownReportPage(reportQuery.currentPage);
	}
	const runReport = () => {
		if (organization === undefined) {
			return;
		}
		if (lastReportRun !== undefined && !isReportOutdated) {
			void reportQuery.refetch();
			return;
		}
		setReportRun({
			organizationId: organization.id,
			days,
			filter: appliedReportFilter,
		});
		updateSearchParams((params) => params.delete("page"));
	};

	const cancelReport = () => {
		if (lastReportRun === undefined) {
			return;
		}
		void queryClient.cancelQueries({
			queryKey: organizationAISpendReportsScopeKey(
				lastReportRun.organizationId,
			),
		});
		// A canceled first page has no rows to fall back to.
		if (reportQuery.data === undefined) {
			setReportRun(undefined);
		} else if (reportQuery.isPlaceholderData) {
			setSearchParamValues({ page: String(shownReportPage) });
		}
	};

	const exportReportMutation = useMutation({
		...exportOrganizationAISpend(queryClient),
		onError: (error) => {
			toast.error("Failed to export the report.", {
				description: getErrorDetail(error),
			});
		},
	});
	const exportReport = () => {
		if (lastReportRun === undefined || organization === undefined) {
			return;
		}
		const { startDate, endDate } = lastReportRun.days;
		exportReportMutation.mutate(
			{
				organizationId: lastReportRun.organizationId,
				period: toSpendPeriod(lastReportRun.days),
				filter: toReportFilter(lastReportRun.filter),
			},
			{
				onSuccess: (file) =>
					saveAs(
						file,
						`ai-spend-export-${organization.name}-${startDate}-to-${endDate}.csv`,
					),
			},
		);
	};

	const openDetailedReport = (username?: string) => {
		updateSearchParams((params) => {
			params.set(tabSearchParam, "detailed-report");
			if (username !== undefined) {
				params.set(useFilterParamsKey, `user:${username}`);
			}
		});
	};

	const tab: AISpendTab =
		searchParams.get(tabSearchParam) === "detailed-report"
			? "detailed-report"
			: "overview";

	const overviewQuery = useQuery({
		...organizationAISpendOverview(
			organization?.id ?? "",
			period,
			TOP_SPENDERS_COUNT,
		),
		enabled: isSpendReady,
	});

	// The unpriced models are slower to compute, so the total renders first and
	// the "Models not set up" warning follows.
	const unpricedModelsQuery = useQuery({
		...organizationUnpricedAIModels(
			organization?.id ?? "",
			period,
			UNPRICED_MODELS_COUNT,
		),
		enabled: isSpendReady,
	});

	// The details need the overview's user IDs, so the list renders first and
	// the unpriced model warnings follow.
	const topSpenderIds =
		overviewQuery.data?.users.map((user) => user.user_id) ?? [];
	const topSpenderDetailsQuery = useQuery({
		...organizationAISpendUsers(organization?.id ?? "", period, topSpenderIds),
		enabled: organization !== undefined && topSpenderIds.length > 0,
	});

	return (
		<>
			<title>{pageTitle("AI spend", "AI settings")}</title>
			<AISpendPageView
				isEntitled={isEntitled}
				isEnabled={isEnabled}
				now={now}
				organizations={organizationsQuery.data ?? []}
				organization={organization}
				onOrganizationChange={(next) => {
					// Only switching to another organization starts the report filter over.
					if (next.id === organization?.id) {
						return;
					}
					setSearchParamValues({
						[modelOrganizationSearchParam]: next.name,
						[useFilterParamsKey]: undefined,
					});
				}}
				tab={tab}
				onTabChange={(next) => setSearchParamValues({ [tabSearchParam]: next })}
				isOrganizationsLoading={organizationsQuery.isLoading}
				organizationsError={organizationsQuery.error}
				days={days}
				onDaysChange={(next) =>
					setSearchParamValues({
						[startDateSearchParam]: next.startDate,
						[endDateSearchParam]: next.endDate,
					})
				}
				minDay={minDay}
				reportFilter={reportFilter.query}
				onReportFilterChange={reportFilter.update}
				reportQuery={reportQuery}
				hasRunReport={lastReportRun !== undefined}
				isReportOutdated={isReportOutdated}
				onRunReport={runReport}
				onCancelReport={cancelReport}
				canExportReport={(reportQuery.data?.count ?? 0) > 0}
				isExportingReport={exportReportMutation.isPending}
				onExportReport={exportReport}
				onOpenDetailedReport={openDetailedReport}
				overview={overviewQuery.data}
				overviewError={overviewQuery.error}
				unpricedModels={unpricedModelsQuery.data}
				isUnpricedModelsLoading={unpricedModelsQuery.isLoading}
				topSpenderDetails={topSpenderDetailsQuery.data?.users}
				topSpenderDetailsError={topSpenderDetailsQuery.error}
				canViewSessions={permissions.viewAnyAIBridgeInterception}
			/>
		</>
	);
};

export default AISpendPage;
