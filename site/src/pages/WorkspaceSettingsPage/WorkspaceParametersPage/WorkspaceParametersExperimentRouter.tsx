import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	optOutKey,
	useDynamicParametersOptOut,
} from "modules/workspaces/DynamicParameter/useDynamicParametersOptOut";
import type { FC } from "react";
import { ExperimentalFormContext } from "../../CreateWorkspacePage/ExperimentalFormContext";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import WorkspaceParametersPage from "./WorkspaceParametersPage";
import WorkspaceParametersPageExperimental from "./WorkspaceParametersPageExperimental";

const WorkspaceParametersExperimentRouter: FC = () => {
	const { experiments } = useDashboard();
	const workspace = useWorkspaceSettings();
	const isDynamicParametersEnabled = experiments.includes("dynamic-parameters");

	const optOutQuery = useDynamicParametersOptOut({
		templateId: workspace.template_id,
		templateUsesClassicParameters:
			workspace.template_use_classic_parameter_flow,
		enabled: isDynamicParametersEnabled,
	});

	if (isDynamicParametersEnabled) {
		if (optOutQuery.isError) {
			return <ErrorAlert error={optOutQuery.error} />;
		}
		if (!optOutQuery.data) {
			return <Loader />;
		}

		const toggleOptedOut = () => {
			const key = optOutKey(optOutQuery.data.templateId);
			const storedValue = localStorage.getItem(key);

			const current = storedValue
				? storedValue === "true"
				: Boolean(workspace.template_use_classic_parameter_flow);

			localStorage.setItem(key, (!current).toString());
			optOutQuery.refetch();
		};

		return (
			<ExperimentalFormContext.Provider value={{ toggleOptedOut }}>
				{optOutQuery.data.optedOut ? (
					<WorkspaceParametersPage />
				) : (
					<WorkspaceParametersPageExperimental />
				)}
			</ExperimentalFormContext.Provider>
		);
	}

	return <WorkspaceParametersPage />;
};

export default WorkspaceParametersExperimentRouter;
