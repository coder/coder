import { usePermissions } from "hooks/usePermissions";
import { DeploymentBannerView } from "./DeploymentBannerView";
import { useQuery } from "react-query";
import { deploymentStats } from "api/queries/deployment";

export const DeploymentBanner: React.FC = () => {
  const permissions = usePermissions();
  const deploymentStatsQuery = useQuery(deploymentStats());

  if (!permissions.viewDeploymentValues || !deploymentStatsQuery.data) {
    return null;
  }

  return (
    <DeploymentBannerView
      stats={deploymentStatsQuery.data}
      fetchStats={() => deploymentStatsQuery.refetch()}
    />
  );
};
