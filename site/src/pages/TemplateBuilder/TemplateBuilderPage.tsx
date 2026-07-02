import type { FC } from "react";
import { useMutation, useQuery } from "react-query";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import {
	createTemplateFromBuilder,
	templateBuilderBases,
} from "#/api/queries/templateBuilder";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { pageTitle } from "#/utils/page";
import { TemplateBuilderPageView } from "./TemplateBuilderPageView";
import type { TemplateBuilderWizardState } from "./wizardState";
import { toCreateTemplateRequest, toSelectedBaseMeta } from "./wizardState";

const TemplateBuilderPage: FC = () => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const { permissions } = useAuthenticated();
	const [searchParams] = useSearchParams();
	const { data, error, isLoading } = useQuery(deploymentConfig());
	const createMutation = useMutation(createTemplateFromBuilder());

	const builderDisabled = data?.config?.template_builder?.disabled ?? false;

	const basesQuery = useQuery({
		...templateBuilderBases(),
		enabled: !builderDisabled && !isLoading && permissions.createTemplates,
	});

	const baseParam = searchParams.get("base");

	// Wait for bases to load when a base query param is present so we can
	// resolve it before the wizard mounts and initializes its state.
	if (isLoading || (baseParam && basesQuery.isLoading)) {
		return <Loader />;
	}

	if (!permissions.createTemplates) {
		return <Navigate to="/templates" replace />;
	}

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

	// Resolve the preselected base from the query param, if present.
	const preselectedBase = baseParam
		? basesQuery.data?.bases?.find((b) => b.id === baseParam)
		: undefined;

	return (
		<>
			<title>{pageTitle("Create Template")}</title>
			<TemplateBuilderPageView
				error={error}
				basesData={basesQuery.data}
				preselectedBase={
					preselectedBase ? toSelectedBaseMeta(preselectedBase) : undefined
				}
				onCreateTemplate={handleCreate}
				createError={createMutation.error}
				isCreating={createMutation.isPending}
				onClearCreateError={() => createMutation.reset()}
			/>
		</>
	);
};

export default TemplateBuilderPage;
