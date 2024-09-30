import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const ObservabilitySettingsPage: FC = () => {
	const { deploymentValues } = useManagementSettings();
	const { entitlements } = useDashboard();
	const { multiple_organizations: hasPremiumLicense } = useFeatureVisibility();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Observability Settings")}</title>
			</Helmet>

			{deploymentValues ? (
				<ObservabilitySettingsPageView
					options={deploymentValues.options}
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
