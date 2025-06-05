import { templateByName } from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import CreateWorkspacePage from "./CreateWorkspacePage";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";
import { ExperimentalFormContext } from "./ExperimentalFormContext";

const CreateWorkspaceExperimentRouter: FC = () => {
	const { experiments } = useDashboard();
	const dynamicParametersEnabled = experiments.includes("dynamic-parameters");

	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const templateQuery = useQuery({
		...templateByName(organizationName, templateName),
		enabled: dynamicParametersEnabled,
	});

	const optOutQuery = useQuery({
		enabled: !!templateQuery.data,
		queryKey: [organizationName, "template", templateQuery.data?.id, "optOut"],
		queryFn: () => {
			const templateId = templateQuery.data?.id;
			const localStorageKey = optOutKey(templateId ?? "");
			const storedOptOutString = localStorage.getItem(localStorageKey);

			let optOutResult: boolean;

			if (storedOptOutString !== null) {
				optOutResult = storedOptOutString === "true";
			} else {
				optOutResult = !!templateQuery.data?.use_classic_parameter_flow;
			}

			return {
				templateId: templateId,
				optedOut: optOutResult,
			};
		},
	});

	if (dynamicParametersEnabled) {
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
	}

	return <CreateWorkspacePage />;
};

export default CreateWorkspaceExperimentRouter;

const optOutKey = (id: string) => `parameters.${id}.optOut`;
