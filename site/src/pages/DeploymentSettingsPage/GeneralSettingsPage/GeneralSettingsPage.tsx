import { deploymentDAUs } from "api/queries/deployment";
import { entitlements } from "api/queries/entitlements";
import { availableExperiments, experiments } from "api/queries/experiments";
import { Loader } from "components/Loader/Loader";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { GeneralSettingsPageView } from "./GeneralSettingsPageView";

const GeneralSettingsPage: FC = () => {
	const { deploymentValues } = useManagementSettings();
	const deploymentDAUsQuery = useQuery(deploymentDAUs());
	const safeExperimentsQuery = useQuery(availableExperiments());

	const { metadata } = useEmbeddedMetadata();
	const entitlementsQuery = useQuery(entitlements(metadata.entitlements));
	const enabledExperimentsQuery = useQuery(experiments(metadata.experiments));

	const safeExperiments = safeExperimentsQuery.data?.safe ?? [];
	const invalidExperiments =
		enabledExperimentsQuery.data?.filter((exp) => {
			return !safeExperiments.includes(exp);
		}) ?? [];

	return (
		<>
			<Helmet>
				<title>{pageTitle("General Settings")}</title>
			</Helmet>
			{deploymentValues ? (
				<GeneralSettingsPageView
					deploymentOptions={deploymentValues.options}
					deploymentDAUs={deploymentDAUsQuery.data}
					deploymentDAUsError={deploymentDAUsQuery.error}
					entitlements={entitlementsQuery.data}
					invalidExperiments={invalidExperiments}
					safeExperiments={safeExperiments}
				/>
			) : (
				<Loader />
			)}
		</>
	);
};

export default GeneralSettingsPage;
