import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useQuery } from "react-query";
import { ExperimentalFormContext } from "../../CreateWorkspacePage/ExperimentalFormContext";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import WorkspaceParametersPage from "./WorkspaceParametersPage";
import WorkspaceParametersPageExperimental from "./WorkspaceParametersPageExperimental";

const WorkspaceParametersExperimentRouter: FC = () => {
	const { experiments } = useDashboard();
	const workspace = useWorkspaceSettings();
	const dynamicParametersEnabled = experiments.includes("dynamic-parameters");

	const optOutQuery = useQuery(
		dynamicParametersEnabled
			? {
					queryKey: [
						"workspace",
						workspace.id,
						"template_id",
						workspace.template_id,
						"optOut",
					],
					queryFn: () => ({
						templateId: workspace.template_id,
						workspaceId: workspace.id,
						optedOut:
							localStorage.getItem(optOutKey(workspace.template_id)) === "true",
					}),
				}
			: { enabled: false },
	);

	if (dynamicParametersEnabled) {
		if (optOutQuery.isLoading) {
			return <Loader />;
		}
		if (!optOutQuery.data) {
			return <ErrorAlert error={optOutQuery.error} />;
		}

		const toggleOptedOut = () => {
			const key = optOutKey(optOutQuery.data.templateId);
			const current = localStorage.getItem(key) === "true";
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

const optOutKey = (id: string) => `parameters.${id}.optOut`;
