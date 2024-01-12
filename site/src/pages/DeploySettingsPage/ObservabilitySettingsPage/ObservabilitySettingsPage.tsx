import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { useDeploySettings } from "../DeploySettingsLayout";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const ObservabilitySettingsPage: FC = () => {
  const { deploymentValues: deploymentValues } = useDeploySettings();
  const { entitlements } = useDashboard();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Observability Settings")}</title>
      </Helmet>

      <ObservabilitySettingsPageView
        options={deploymentValues.options}
        featureAuditLogEnabled={entitlements.features["audit_log"].enabled}
      />
    </>
  );
};

export default ObservabilitySettingsPage;
