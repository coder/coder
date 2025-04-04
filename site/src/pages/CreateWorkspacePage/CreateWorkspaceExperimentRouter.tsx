import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import CreateWorkspacePage from "./CreateWorkspacePage";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";

const CreateWorkspaceExperimentRouter: FC = () => {
	const { experiments } = useDashboard();

	const dynamicParametersEnabled = experiments.includes("dynamic-parameters");

	if (dynamicParametersEnabled) {
		return <CreateWorkspacePageExperimental />;
	}

	return <CreateWorkspacePage />;
};

export default CreateWorkspaceExperimentRouter;
