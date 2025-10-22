import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { pageTitle } from "utils/page";
import { PremiumPageView } from "./PremiumPageView";

const PremiumPage: FC = () => {
	const { entitlements } = useDashboard();

	return (
		<>
			<title>{pageTitle("Premium Features")}</title>

			<PremiumPageView isEnterprise={entitlements.has_license} />
		</>
	);
};

export default PremiumPage;
