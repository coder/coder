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

	const optOutQuery = useQuery({
		enabled: dynamicParametersEnabled,
		queryKey: [
			"workspace",
			workspace.id,
			"template_id",
			workspace.template_id,
			"optOut",
		],
		queryFn: () => {
			const templateId = workspace.template_id;
			const workspaceId = workspace.id;
			const localStorageKey = optOutKey(templateId);
			const storedOptOutString = localStorage.getItem(localStorageKey);

			let optOutResult: boolean;

			if (storedOptOutString !== null) {
				optOutResult = storedOptOutString === "true";
			} else {
				optOutResult = Boolean(workspace.template_use_classic_parameter_flow);
			}

			return {
				templateId,
				workspaceId,
				optedOut: optOutResult,
			};
		},
	});

	if (dynamicParametersEnabled) {
		if (optOutQuery.isLoading) {
			return <Loader />;
		}
		if (!optOutQuery.data) {
			return <ErrorAlert error={optOutQuery.error} />;
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

const optOutKey = (id: string) => `parameters.${id}.optOut`;
