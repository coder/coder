import { type FC, useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import {
	createTemplateFromBuilder,
	recordTemplateBuilderSession,
	templateBuilderBases,
} from "#/api/queries/templateBuilder";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { pageTitle } from "#/utils/page";
import { generateUUID } from "#/utils/random";
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
	const sessionMutation = useMutation(recordTemplateBuilderSession());

	// Stable session ID for the lifetime of this page mount, shared
	// across wizard_entry and compose_completion telemetry events.
	const sessionId = useMemo(() => generateUUID(), []);

	const builderDisabled = data?.config?.template_builder?.disabled ?? false;
	const wizardReady =
		!builderDisabled && !isLoading && permissions.createTemplates;

	// Report wizard_entry once the builder is ready and accessible.
	const reportEntry = useCallback(() => {
		sessionMutation.mutate({
			session_id: sessionId,
			event_type: "wizard_entry",
		});
	}, [sessionMutation.mutate, sessionId]);

	// Report compose_completion when the create request settles. Duration
	// is captured at submit time so it measures wizard usage, not the
	// create request round trip.
	const reportCompletion = useCallback(
		(
			state: TemplateBuilderWizardState,
			success: boolean,
			durationSeconds: number,
		) => {
			sessionMutation.mutate({
				session_id: state.sessionId,
				event_type: "compose_completion",
				base_template_id: state.baseTemplateId ?? undefined,
				module_ids: state.modules.map((m) => m.id),
				duration_seconds: durationSeconds,
				success,
			});
		},
		[sessionMutation.mutate],
	);

	useEffect(() => {
		if (!wizardReady) {
			return;
		}
		reportEntry();
	}, [wizardReady, reportEntry]);

	const basesQuery = useQuery({
		...templateBuilderBases(),
		enabled: wizardReady,
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
		const durationSeconds = (Date.now() - state.enteredAt) / 1000;

		createMutation.mutate(req, {
			onSuccess: (resp) => {
				reportCompletion(state, true, durationSeconds);
				const t = resp.template;
				navigate(
					`${getLink(linkToTemplate(t.organization_name, t.name))}/files`,
					{ state: { justCreated: true } },
				);
			},
			onError: () => {
				reportCompletion(state, false, durationSeconds);
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
				isCreating={createMutation.isPending || createMutation.isSuccess}
				onClearCreateError={() => createMutation.reset()}
				sessionId={sessionId}
			/>
		</>
	);
};

export default TemplateBuilderPage;
