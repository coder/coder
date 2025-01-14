import { deploymentDAUs } from "api/queries/deployment";
import { entitlements } from "api/queries/entitlements";
import { availableExperiments, experiments } from "api/queries/experiments";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { GeneralSettingsPageView } from "./GeneralSettingsPageView";
import { insightsUserStatusCounts } from "api/queries/insights";
import type { UserStatusChangeCount } from "api/typesGenerated";
import { eachDayOfInterval, isSameDay } from "date-fns";

const GeneralSettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();
	const safeExperimentsQuery = useQuery(availableExperiments());

	const { metadata } = useEmbeddedMetadata();
	const entitlementsQuery = useQuery(entitlements(metadata.entitlements));
	const enabledExperimentsQuery = useQuery(experiments(metadata.experiments));

	const safeExperiments = safeExperimentsQuery.data?.safe ?? [];
	const invalidExperiments =
		enabledExperimentsQuery.data?.filter((exp) => {
			return !safeExperiments.includes(exp);
		}) ?? [];

	const { data: userStatusCount } = useQuery(insightsUserStatusCounts());

	return (
		<>
			<Helmet>
				<title>{pageTitle("General Settings")}</title>
			</Helmet>
			<GeneralSettingsPageView
				deploymentOptions={deploymentConfig.options}
				activeUsersCount={normalizeStatusCount(userStatusCount?.active)}
				entitlements={entitlementsQuery.data}
				invalidExperiments={invalidExperiments}
				safeExperiments={safeExperiments}
			/>
		</>
	);
};

// TODO: Remove this function once the API returns values sorted by date and
// includes all dates within the specified range. The
// `/api/v2/insights/user-status-counts` endpoint does not return the
// `UserStatusChangeCount[]` items sorted by date, nor does it backfill missing
// dates within the specified range.
function normalizeStatusCount(
	statusCount: UserStatusChangeCount[] | undefined,
) {
	if (!statusCount) {
		return undefined;
	}

	const sortedCounts = statusCount.toSorted((a, b) => {
		return new Date(a.date).getTime() - new Date(b.date).getTime();
	});

	const dates = eachDayOfInterval({
		start: new Date(sortedCounts[0].date),
		end: new Date(sortedCounts[sortedCounts.length - 1].date),
	});

	const backFilledCounts: UserStatusChangeCount[] = [];
	dates.forEach((date, i) => {
		const existingCount = sortedCounts.find((c) =>
			isSameDay(date, new Date(c.date)),
		);
		if (existingCount) {
			backFilledCounts.push(existingCount);
		} else {
			backFilledCounts.push({
				date: date.toISOString(),
				count: backFilledCounts[i - 1].count,
			});
		}
	});

	return backFilledCounts;
}

export default GeneralSettingsPage;
