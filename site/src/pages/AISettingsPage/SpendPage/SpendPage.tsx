import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import {
	aiSpendOrganizations,
	paginatedOrganizationAISpend,
} from "#/api/queries/aiBridge";
import type { OrganizationAISpendFilter } from "#/api/typesGenerated";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useClientFilterMenu } from "#/pages/AIBridgePage/filters/ClientFilter";
import { useModelFilterMenu } from "#/pages/AIBridgePage/filters/ModelFilter";
import { useProviderFilterMenu } from "#/pages/AIBridgePage/filters/ProviderFilter";
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { SpendPageView } from "./SpendPageView";
import { defaultSpendPeriod } from "./spendPeriod";

const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";

type SpendDimensions = Pick<
	OrganizationAISpendFilter,
	"provider_name" | "client" | "model"
>;

/** Extracts an explicit period from the URL, or null if absent or invalid. */
const parsePeriod = (
	searchParams: URLSearchParams,
): Pick<DateTimeRangeValue, "start" | "end"> | null => {
	const startParam = searchParams.get(startDateSearchParam)?.trim();
	const endParam = searchParams.get(endDateSearchParam)?.trim();
	if (!startParam || !endParam) {
		return null;
	}
	const start = new Date(startParam);
	const end = new Date(endParam);
	if (
		Number.isNaN(start.getTime()) ||
		Number.isNaN(end.getTime()) ||
		start.getTime() >= end.getTime()
	) {
		return null;
	}
	return { start, end };
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
		searchParams.get(modelOrganizationSearchParam),
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

	// The default period lives in memory, not the URL, so a shared link
	// resolves relative to the viewer's current time. It is fixed per mount so
	// query cache keys stay stable.
	const [defaultPeriod] = useState(() => defaultSpendPeriod(now ?? new Date()));
	const explicitPeriod = parsePeriod(searchParams);
	const period = explicitPeriod ?? defaultPeriod;

	// The preset is display-only; the URL stores resolved timestamps. Show the
	// last picked preset only while the URL range still matches it.
	const [lastPicked, setLastPicked] = useState<DateTimeRangeValue>();
	const preset =
		explicitPeriod === null
			? defaultPeriod.preset
			: lastPicked?.preset !== undefined &&
					lastPicked.start.getTime() === period.start.getTime() &&
					lastPicked.end.getTime() === period.end.getTime()
				? lastPicked.preset
				: undefined;

	const spendFilter: OrganizationAISpendFilter = {
		period_start: period.start.toISOString(),
		period_end: period.end.toISOString(),
		...dimensions,
	};

	const onPeriodChange = (value: DateTimeRangeValue) => {
		setLastPicked(value);
		setFilterParams({
			[startDateSearchParam]: value.start.toISOString(),
			[endDateSearchParam]: value.end.toISOString(),
		});
	};

	const reportQuery = usePaginatedQuery({
		...paginatedOrganizationAISpend(organization?.id ?? "", spendFilter),
		recordsPerPage: 10,
		preventScrollReset: true,
		enabled: isSpendAvailable && organization !== undefined,
	});

	// Retention is deployment-wide. Keep the last reported cutoff while the
	// next report loads so the picker keeps hiding periods the server rejects.
	const [retention, setRetention] = useState<{ start: string | undefined }>();
	if (
		reportQuery.data &&
		!reportQuery.isPlaceholderData &&
		(retention === undefined ||
			retention.start !== reportQuery.data.retention_start)
	) {
		setRetention({ start: reportQuery.data.retention_start });
	}
	const minDate = retention?.start ? new Date(retention.start) : undefined;

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
				period={{ ...period, preset }}
				minDate={minDate}
				onPeriodChange={onPeriodChange}
				filterMenus={canFilterDimensions ? filterMenus : undefined}
				reportQuery={reportQuery}
			/>
		</>
	);
};

export default SpendPage;
