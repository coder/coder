import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";

const ObservabilitySettingsPage: FC = () => {
	const { deploymentConfig } = useDeploymentSettings();
	const { entitlements } = useDashboard();
	const { multiple_organizations: hasPremiumLicense } = useFeatureVisibility();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Observability Settings")}</title>
			</Helmet>

			{deploymentConfig ? (
				<ObservabilitySettingsPageView
					options={deploymentConfig.options}
					featureAuditLogEnabled={entitlements.features.audit_log.enabled}
					isPremium={hasPremiumLicense}
				/>
			) : (
				<Loader />
			)}
		</>
	);
};

export default ObservabilitySettingsPage;
