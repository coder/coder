import { templateByName } from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import CreateWorkspacePage from "./CreateWorkspacePage";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";

const CreateWorkspaceExperimentRouter: FC = () => {
	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const templateQuery = useQuery(
		templateByName(organizationName, templateName),
	);

	if (templateQuery.isError) {
		return <ErrorAlert error={templateQuery.error} />;
	}
	if (!templateQuery.data) {
		return <Loader />;
	}

	return (
		<>
			{templateQuery.data?.use_classic_parameter_flow ? (
				<CreateWorkspacePage />
			) : (
				<CreateWorkspacePageExperimental />
			)}
		</>
	);
};

export default CreateWorkspaceExperimentRouter;
