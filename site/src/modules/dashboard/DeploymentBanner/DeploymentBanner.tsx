import { health } from "api/queries/debug";
import { deploymentStats } from "api/queries/deployment";
import { useAuthenticated } from "hooks";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useLocation } from "react-router";
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

export const DeploymentBanner: FC = () => {
	const { permissions } = useAuthenticated();
	const deploymentStatsQuery = useQuery(deploymentStats());
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
		/>
	);
};
