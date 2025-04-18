import { templateByName } from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, createContext } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import CreateWorkspacePage from "./CreateWorkspacePage";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";

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

	const optOutQuery = useQuery(
		templateQuery.data
			? {
					queryKey: [
						organizationName,
						"template",
						templateQuery.data.id,
						"optOut",
					],
					queryFn: () => ({
						templateId: templateQuery.data.id,
						optedOut:
							localStorage.getItem(optOutKey(templateQuery.data.id)) === "true",
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
			<NewFormContext.Provider value={{ toggleOptedOut }}>
				{optOutQuery.data.optedOut ? (
					<CreateWorkspacePage />
				) : (
					<CreateWorkspacePageExperimental />
				)}
			</NewFormContext.Provider>
		);
	}

	return <CreateWorkspacePage />;
};

export default CreateWorkspaceExperimentRouter;

const optOutKey = (id: string) => `parameters.${id}.optOut`;

export const NewFormContext = createContext<
	{ toggleOptedOut: () => void } | undefined
>(undefined);
