import type { FC } from "react";
import { useMutation, useQuery } from "react-query";
import { Navigate, useNavigate } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import {
	createTemplateFromBuilder,
	templateBuilderBases,
} from "#/api/queries/templateBuilder";
import { Loader } from "#/components/Loader/Loader";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { pageTitle } from "#/utils/page";
import { TemplateBuilderPageView } from "./TemplateBuilderPageView";
import type { TemplateBuilderWizardState } from "./wizardState";
import { toCreateTemplateRequest } from "./wizardState";

const TemplateBuilderPage: FC = () => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const { data, error, isLoading } = useQuery(deploymentConfig());
	const basesQuery = useQuery(templateBuilderBases());
	const createMutation = useMutation(createTemplateFromBuilder());

	if (isLoading) {
		return <Loader />;
	}

	// If the template builder is disabled in the deployment config,
	// redirect to the new template page.
	const builderDisabled = data?.config?.template_builder?.disabled ?? false;
	if (builderDisabled) {
		return <Navigate to="/templates/new" replace />;
	}

	const handleCreate = (state: TemplateBuilderWizardState) => {
		const req = toCreateTemplateRequest(state);
		createMutation.mutate(req, {
			onSuccess: (resp) => {
				const t = resp.template;
				navigate(
					`${getLink(linkToTemplate(t.organization_name, t.name))}/files`,
					{ state: { justCreated: true } },
				);
			},
		});
	};

	return (
		<>
			<title>{pageTitle("Create Template")}</title>
			<TemplateBuilderPageView
				error={error}
				basesData={basesQuery.data}
				onCreateTemplate={handleCreate}
				createError={createMutation.error}
				isCreating={createMutation.isPending}
			/>
		</>
	);
};

export default TemplateBuilderPage;
