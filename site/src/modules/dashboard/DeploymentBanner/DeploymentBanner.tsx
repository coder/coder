import dayjs from "dayjs";
import { useQuery } from "react-query";
import { useLocation } from "react-router";
import { health } from "#/api/queries/debug";
import { deploymentStats } from "#/api/queries/deployment";
import type { DeploymentStats } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { DeploymentBannerView } from "./DeploymentBannerView";

const HIDE_DEPLOYMENT_BANNER_PATHS = [
	// Hide the banner on workspace page because it already has a lot of
	// information.
	// - It adds names to the main groups that we're checking for, so it'll be a
	//   little more self-documenting
	// - It redefines each group to only allow the characters A-Z (lowercase or
	//   uppercase), numbers, and hyphens
	/^\/@(?<username>[a-zA-Z0-9-]+)\/(?<workspace_name>[a-zA-Z0-9-]+)$/,
];

export const DeploymentBanner: React.FC = () => {
	const { permissions } = useAuthenticated();
	// Refetches stop while the tab is hidden, and stale stats refresh when
	// the tab is focused again.
	const deploymentStatsQuery = useQuery({
		...deploymentStats(),
		enabled: permissions.viewDeploymentStats,
		refetchInterval: ({ state }) =>
			state.data ? getRefreshInterval(state.data) : false,
		staleTime: ({ state }) => (state.data ? getRefreshInterval(state.data) : 0),
		refetchOnWindowFocus: true,
	});
	const healthQuery = useQuery({
		...health(),
		enabled: permissions.viewDeploymentConfig,
	});
	const location = useLocation();
	const isHidden = HIDE_DEPLOYMENT_BANNER_PATHS.some((regex) =>
		regex.test(location.pathname),
	);

	if (
		isHidden ||
		!permissions.viewDeploymentConfig ||
		!deploymentStatsQuery.data
	) {
		return null;
	}

	return (
		<DeploymentBannerView
			health={healthQuery.data}
			stats={deploymentStatsQuery.data}
			fetchStats={() => deploymentStatsQuery.refetch()}
			refreshAt={
				deploymentStatsQuery.dataUpdatedAt +
				getRefreshInterval(deploymentStatsQuery.data)
			}
		/>
	);
};

const getRefreshInterval = (stats: DeploymentStats): number => {
	const seconds = dayjs(stats.next_update_at).diff(
		stats.collected_at,
		"seconds",
	);
	return Math.max(seconds, 1) * 1000;
};
