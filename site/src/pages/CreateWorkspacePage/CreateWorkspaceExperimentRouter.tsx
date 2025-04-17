import { useDashboard } from "modules/dashboard/useDashboard";
import { createContext, type FC, useState } from "react";
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

		const optOut = `parameters.${templateQuery.data.id}.optOut`;
		const [optedOut, setOptedOut] = useState(
			localStorage.getItem(optOut) == "true",
		);

		const toggleOptedOut = () => {
			setOptedOut((prev) => {
				const next = !prev;
				localStorage.setItem(optOut, next.toString());
				return next;
			});
		};

		return (
			<CreateWorkspaceContext.Provider value={{ toggleOptedOut }}>
				{optedOut ? (
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

const CreateWorkspaceContext = createContext<
	{ toggleOptedOut: () => void } | undefined
>(undefined);
