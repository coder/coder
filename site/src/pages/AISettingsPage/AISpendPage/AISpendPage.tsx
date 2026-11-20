import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import {
	aiSpendOrganizations,
	organizationAISpendOverview,
	organizationUnpricedAIModels,
} from "#/api/queries/aiBridge";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useUpdateSearchParams } from "#/hooks/useUpdateSearchParams";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { AISpendPageView } from "./AISpendPageView";
import { defaultSpendDays, parseSpendDays, toSpendPeriod } from "./spendDays";
import { TOP_SPENDERS_COUNT } from "./topSpenders";

const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";

type AISpendPageProps = {
	now?: Date;
};

const AISpendPage: React.FC<AISpendPageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const { isEntitled, isEnabled } = getAIBridgePermissions(
		entitlements,
		permissions,
	);
	const isSpendAvailable = isEntitled && isEnabled;

	const [searchParams] = useSearchParams();
	const updateSearchParams = useUpdateSearchParams();
	const setSearchParamValues = (values: Record<string, string>) => {
		updateSearchParams((params) => {
			for (const [key, value] of Object.entries(values)) {
				params.set(key, value);
			}
		});
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

	// The default range stays out of the URL, so a shared link without dates
	// resolves against the viewer's last 7 days.
	const days =
		parseSpendDays(
			searchParams.get(startDateSearchParam),
			searchParams.get(endDateSearchParam),
		) ?? defaultSpendDays(now ?? new Date());
	const period = toSpendPeriod(days);

	const overviewQuery = useQuery({
		...organizationAISpendOverview(
			organization?.id ?? "",
			period,
			TOP_SPENDERS_COUNT,
		),
		enabled: isSpendAvailable && organization !== undefined,
	});

	// The unpriced models are slower to compute, so the total renders first and
	// the "Models not set up" warning follows.
	const unpricedModelsQuery = useQuery({
		...organizationUnpricedAIModels(organization?.id ?? "", period),
		enabled: isSpendAvailable && organization !== undefined,
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
				onOrganizationChange={(next) =>
					setSearchParamValues({ [modelOrganizationSearchParam]: next.name })
				}
				isOrganizationsLoading={organizationsQuery.isLoading}
				organizationsError={organizationsQuery.error}
				days={days}
				onDaysChange={(next) =>
					setSearchParamValues({
						[startDateSearchParam]: next.startDate,
						[endDateSearchParam]: next.endDate,
					})
				}
				overview={overviewQuery.data}
				overviewError={overviewQuery.error}
				unpricedModels={unpricedModelsQuery.data?.models}
				isUnpricedModelsLoading={unpricedModelsQuery.isLoading}
				canViewSessions={permissions.viewAnyAIBridgeInterception}
			/>
		</>
	);
};

export default AISpendPage;
