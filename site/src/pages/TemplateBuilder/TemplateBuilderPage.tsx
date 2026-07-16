import { type FC, useEffect, useState } from "react";
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
import type {
	SelectedBaseMeta,
	TemplateBuilderWizardState,
} from "./wizardState";
import { toCreateTemplateRequest, toSelectedBaseMeta } from "./wizardState";

const TemplateBuilderPage: FC = () => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const { permissions } = useAuthenticated();
	const [searchParams, setSearchParams] = useSearchParams();
	const { data, error, isLoading } = useQuery(deploymentConfig());
	const createMutation = useMutation(createTemplateFromBuilder());

	const builderDisabled = data?.config?.template_builder?.disabled ?? false;

	const basesQuery = useQuery({
		...templateBuilderBases(),
		enabled: !builderDisabled && !isLoading && permissions.createTemplates,
	});

	// ?base= is the only search param accepted on entry. It is consumed
	// here: resolved against the available bases, stored in local state,
	// and removed from the URL before the wizard mounts.
	const baseParam = searchParams.get("base");
	const [preselectedBase, setPreselectedBase] = useState<SelectedBaseMeta>();
	useEffect(() => {
		if (!baseParam || !basesQuery.data) {
			return;
		}
		const match = basesQuery.data.bases?.find((b) => b.id === baseParam);
		if (match) {
			setPreselectedBase(toSelectedBaseMeta(match));
		}
		const next = new URLSearchParams(searchParams);
		next.delete("base");
		setSearchParams(next, { replace: true });
	}, [baseParam, basesQuery.data, searchParams, setSearchParams]);

	// Hold the wizard until ?base= has been fully consumed so it mounts
	// exactly once with its initial state settled.
	if (isLoading || baseParam) {
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

	return (
		<>
			<title>{pageTitle("Create Template")}</title>
			<TemplateBuilderPageView
				error={error}
				basesData={basesQuery.data}
				preselectedBase={preselectedBase}
				onCreateTemplate={handleCreate}
				createError={createMutation.error}
				isCreating={createMutation.isPending}
				onClearCreateError={() => createMutation.reset()}
			/>
		</>
	);
};

export default TemplateBuilderPage;
