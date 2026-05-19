import type { FC } from "react";
import { useWorkspaceSettings } from "../useWorkspaceSettings";
import WorkspaceParametersPage from "./WorkspaceParametersPage";
import WorkspaceParametersPageExperimental from "./WorkspaceParametersPageExperimental";

const WorkspaceParametersExperimentRouter: FC = () => {
	const { workspace } = useWorkspaceSettings();

	return (
		// oxlint-disable-next-line react/jsx-no-useless-fragment -- pre-existing, see follow-up
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
