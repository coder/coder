import { health } from "api/queries/debug";
import { deploymentStats } from "api/queries/deployment";
import { useAuthenticated } from "hooks";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { useLocation } from "react-router-dom";
import { DeploymentBannerView } from "./DeploymentBannerView";

const HIDE_DEPLOYMENT_BANNER_PATHS = [
	// Workspace page.
	// Hide the banner on workspace page because it already has a lot of information.
	/^\/@[^\/]+\/[^\/]+$/,
];

export const DeploymentBanner: FC = () => {
	const { permissions } = useAuthenticated();
	const deploymentStatsQuery = useQuery(deploymentStats());
	const healthQuery = useQuery({
		...health(),
		enabled: permissions.viewDeploymentConfig,
	});
	const location = useLocation();
	const [visible, setVisible] = useState(true);

	useEffect(() => {
		const isHidden = HIDE_DEPLOYMENT_BANNER_PATHS.some((regex) =>
			regex.test(location.pathname),
		);
		setVisible(!isHidden);
	}, [location.pathname]);

	if (
		!visible ||
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
