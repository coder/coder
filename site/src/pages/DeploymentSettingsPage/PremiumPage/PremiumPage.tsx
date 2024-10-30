import { Loader } from "components/Loader/Loader";
import { useDeploymentSettings } from "modules/management/DeploymentSettingsProvider";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { PremiumPageView } from "./PremiumPageView";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";

const PremiumPage: FC = () => {
	const { multiple_organizations: hasPremiumLicense } = useFeatureVisibility();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Premium Features")}</title>
			</Helmet>
			<PremiumPageView isPremium={hasPremiumLicense} />
		</>
	);
};

export default PremiumPage;
