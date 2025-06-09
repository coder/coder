import { templateByName } from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import {
	optOutKey,
	useDynamicParametersOptOut,
} from "modules/workspaces/DynamicParameter/useDynamicParametersOptOut";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import CreateWorkspacePage from "./CreateWorkspacePage";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";
import { ExperimentalFormContext } from "./ExperimentalFormContext";

const CreateWorkspaceExperimentRouter: FC = () => {
	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const templateQuery = useQuery(templateByName(organizationName, templateName));

	const optOutQuery = useDynamicParametersOptOut({
		templateId: templateQuery.data?.id,
		templateUsesClassicParameters:
			templateQuery.data?.use_classic_parameter_flow,
		enabled: !!templateQuery.data,
	});

	if (templateQuery.isError) {
		return <ErrorAlert error={templateQuery.error} />;
	}
	if (optOutQuery.isError) {
		return <ErrorAlert error={optOutQuery.error} />;
	}
	if (!optOutQuery.data) {
		return <Loader />;
	}

	const toggleOptedOut = () => {
		const key = optOutKey(optOutQuery.data?.templateId ?? "");
		const storedValue = localStorage.getItem(key);

		const current = storedValue
			? storedValue === "true"
			: Boolean(templateQuery.data?.use_classic_parameter_flow);

		localStorage.setItem(key, (!current).toString());
		optOutQuery.refetch();
	};
	
	return (
		<ExperimentalFormContext.Provider value={{ toggleOptedOut }}>
			{optOutQuery.data.optedOut ? (
				<CreateWorkspacePage />
			) : (
				<CreateWorkspacePageExperimental />
			)}
		</ExperimentalFormContext.Provider>
	);
};

export default CreateWorkspaceExperimentRouter;
