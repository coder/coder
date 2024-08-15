import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useDeploySettings } from "../DeploySettingsLayout";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const ObservabilitySettingsPage: FC = () => {
  const { deploymentValues } = useDeploySettings();
  const { entitlements } = useDashboard();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Observability Settings")}</title>
      </Helmet>

      {deploymentValues ? (
        <ObservabilitySettingsPageView
          options={deploymentValues.options}
          featureAuditLogEnabled={entitlements.features.audit_log.enabled}
        />
      ) : (
        <Loader />
      )}
    </>
  );
};

export default ObservabilitySettingsPage;
