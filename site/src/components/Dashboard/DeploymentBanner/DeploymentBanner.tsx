import { type FC } from "react";
import { useQuery } from "react-query";
import { deploymentStats, health } from "api/queries/deployment";
import { usePermissions } from "hooks/usePermissions";
import { DeploymentBannerView } from "./DeploymentBannerView";
import { useDashboard } from "../DashboardProvider";

export const DeploymentBanner: FC = () => {
  const dashboard = useDashboard();
  const permissions = usePermissions();
  const deploymentStatsQuery = useQuery(deploymentStats());
  const healthQuery = useQuery({
    ...health(),
    enabled: dashboard.experiments.includes("deployment_health_page"),
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
