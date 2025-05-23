import { templateByName } from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import { ExperimentalFormContext } from "pages/CreateWorkspacePage/ExperimentalFormContext";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import TemplateEmbedPage from "./TemplateEmbedPage";
import TemplateEmbedPageExperimental from "./TemplateEmbedPageExperimental";

const TemplateEmbedExperimentRouter: FC = () => {
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
					queryFn: () => {
						const templateId = templateQuery.data.id;
						const localStorageKey = optOutKey(templateId);
						const storedOptOutString = localStorage.getItem(localStorageKey);

						let optOutResult: boolean;

						if (storedOptOutString !== null) {
							optOutResult = storedOptOutString === "true";
						} else {
							optOutResult = Boolean(
								templateQuery.data.use_classic_parameter_flow,
							);
						}

						return {
							templateId: templateId,
							optedOut: optOutResult,
						};
					},
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
					<TemplateEmbedPage />
				) : (
					<TemplateEmbedPageExperimental />
				)}
			</ExperimentalFormContext.Provider>
		);
	}

	return <TemplateEmbedPage />;
};

export default TemplateEmbedExperimentRouter;

const optOutKey = (id: string) => `parameters.${id}.optOut`;
