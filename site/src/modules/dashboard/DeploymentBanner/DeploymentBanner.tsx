import { type FC } from "react";
import { useQuery } from "react-query";
import { health } from "api/queries/debug";
import { deploymentStats } from "api/queries/deployment";
import { usePermissions } from "contexts/auth/usePermissions";
import { DeploymentBannerView } from "./DeploymentBannerView";

export const DeploymentBanner: FC = () => {
  const permissions = usePermissions();
  const deploymentStatsQuery = useQuery(deploymentStats());
  const healthQuery = useQuery({
    ...health(),
    enabled: permissions.viewDeploymentValues,
  });

  if (!permissions.viewDeploymentValues || !deploymentStatsQuery.data) {
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
