import type { FC } from "react";
import { useQuery } from "react-query";
import { health } from "api/queries/debug";
import { deploymentStats } from "api/queries/deployment";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { DeploymentBannerView } from "./DeploymentBannerView";

export const DeploymentBanner: FC = () => {
  const { permissions } = useAuthenticated();
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
