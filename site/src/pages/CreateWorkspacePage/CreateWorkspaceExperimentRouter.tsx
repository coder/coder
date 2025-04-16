import { useDashboard } from "modules/dashboard/useDashboard";
import { createContext, type FC } from "react";
import CreateWorkspacePage from "./CreateWorkspacePage";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";
import { useParams } from "react-router-dom";
import { useQuery } from "react-query";
import { templateByName } from "api/queries/templates";

const CreateWorkspaceExperimentRouter: FC = () => {
	const { experiments } = useDashboard();
	const dynamicParametersEnabled = experiments.includes("dynamic-parameters");

	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const templateQuery = useQuery(
		templateByName(organizationName, templateName),
	);

	const something = JSON.parse(
		localStorage.getItem(`parameters.${templateQuery.data?.id}.optOut`) ?? "",
	);

	if (dynamicParametersEnabled) {
		return <CreateWorkspacePageExperimental />;
	}

	return <CreateWorkspacePage />;
};

export default CreateWorkspaceExperimentRouter;

const CreateWorkspaceProvider = createContext(undefined);
