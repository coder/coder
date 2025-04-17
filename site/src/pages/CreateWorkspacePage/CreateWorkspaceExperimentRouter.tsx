import { useDashboard } from "modules/dashboard/useDashboard";
import { createContext, type FC } from "react";
import CreateWorkspacePage from "./CreateWorkspacePage";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";
import { useParams } from "react-router-dom";
import { useQuery } from "react-query";
import { templateByName } from "api/queries/templates";
import { Loader } from "components/Loader/Loader";
import { ErrorAlert } from "components/Alert/ErrorAlert";

const CreateWorkspaceExperimentRouter: FC = () => {
	const { experiments } = useDashboard();
	const dynamicParametersEnabled = experiments.includes("dynamic-parameters");

	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const templateQuery = useQuery(
		dynamicParametersEnabled
			? templateByName(organizationName, templateName)
			: { enabled: false },
	);

	if (dynamicParametersEnabled) {
		if (templateQuery.isLoading) {
			return <Loader />;
		}

		if (!templateQuery.data) {
			return <ErrorAlert error={templateQuery.error} />;
		}

		const hasOptedOut =
			localStorage.Item(`parameters.${templateQuery.data.id}.optOut`) == "true";
		return (
			<CreateWorkspaceContext.Provider value={{}}>
				{hasOptedOut ? (
					<CreateWorkspacePage />
				) : (
					<CreateWorkspacePageExperimental />
				)}
			</CreateWorkspaceContext.Provider>
		);
	}

	return <CreateWorkspacePage />;
};

export default CreateWorkspaceExperimentRouter;

const CreateWorkspaceContext = createContext<{}>({});
