import type { FC } from "react";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import WorkspaceParametersPage from "./WorkspaceParametersPage";
import WorkspaceParametersPageExperimental from "./WorkspaceParametersPageExperimental";

const WorkspaceParametersExperimentRouter: FC = () => {
	const workspace = useWorkspaceSettings();

	return (
		<>
			{workspace.template_use_classic_parameter_flow ? (
				<WorkspaceParametersPage />
			) : (
				<WorkspaceParametersPageExperimental />
			)}
		</>
	);
};

export default WorkspaceParametersExperimentRouter;
