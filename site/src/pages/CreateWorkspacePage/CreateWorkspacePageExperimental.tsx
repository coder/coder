import { API } from "api/api";
import type { ApiErrorResponse } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import {
	richParameters,
	templateByName,
	templateVersionExternalAuth,
	templateVersionPresets,
} from "api/queries/templates";
import { autoCreateWorkspace, createWorkspace } from "api/queries/workspaces";
import type {
	TemplateVersionParameter,
	UserParameter,
	Workspace,
} from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type WorkspacePermissions,
	workspacePermissionChecks,
} from "modules/permissions/workspaces";
import { generateWorkspaceName } from "modules/workspaces/generateWorkspaceName";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import type { AutofillBuildParameter } from "utils/richParameters";
import { paramsUsedToCreateWorkspace } from "utils/workspace";
import { CreateWorkspacePageViewExperimental } from "./CreateWorkspacePageViewExperimental";
export const createWorkspaceModes = ["form", "auto", "duplicate"] as const;
export type CreateWorkspaceMode = (typeof createWorkspaceModes)[number];

export type ExternalAuthPollingState = "idle" | "polling" | "abandoned";

const CreateWorkspacePageExperimental: FC = () => {
	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const { user: me } = useAuthenticated();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const { experiments } = useDashboard();

	const customVersionId = searchParams.get("version") ?? undefined;
	const defaultName = searchParams.get("name");
	const disabledParams = searchParams.get("disable_params")?.split(",");
	const [mode, setMode] = useState(() => getWorkspaceMode(searchParams));
	const [autoCreateError, setAutoCreateError] =
		useState<ApiErrorResponse | null>(null);

	const queryClient = useQueryClient();
	const autoCreateWorkspaceMutation = useMutation(
		autoCreateWorkspace(queryClient),
	);
	const createWorkspaceMutation = useMutation(createWorkspace(queryClient));

	const templateQuery = useQuery(
		templateByName(organizationName, templateName),
	);
	const templateVersionPresetsQuery = useQuery({
		...templateVersionPresets(templateQuery.data?.active_version_id ?? ""),
		enabled: templateQuery.data !== undefined,
	});
	const permissionsQuery = useQuery(
		templateQuery.data
			? checkAuthorization({
					checks: workspacePermissionChecks(templateQuery.data.organization_id),
				})
			: { enabled: false },
	);
	const realizedVersionId =
		customVersionId ?? templateQuery.data?.active_version_id;
	const organizationId = templateQuery.data?.organization_id;
	const richParametersQuery = useQuery({
		...richParameters(realizedVersionId ?? ""),
		enabled: realizedVersionId !== undefined,
	});
	const realizedParameters = richParametersQuery.data
		? richParametersQuery.data.filter(paramsUsedToCreateWorkspace)
		: undefined;

	const {
		externalAuth,
		externalAuthPollingState,
		startPollingExternalAuth,
		isLoadingExternalAuth,
	} = useExternalAuth(realizedVersionId);

	const isLoadingFormData =
		templateQuery.isLoading ||
		permissionsQuery.isLoading ||
		richParametersQuery.isLoading;
	const loadFormDataError =
		templateQuery.error ?? permissionsQuery.error ?? richParametersQuery.error;

	const title = autoCreateWorkspaceMutation.isLoading
		? "Creating workspace..."
		: "Create workspace";

	const onCreateWorkspace = useCallback(
		(workspace: Workspace) => {
			navigate(`/@${workspace.owner_name}/${workspace.name}`);
		},
		[navigate],
	);

	// Auto fill parameters
	const autofillEnabled = experiments.includes("auto-fill-parameters");
	const userParametersQuery = useQuery({
		queryKey: ["userParameters"],
		queryFn: () => API.getUserParameters(templateQuery.data!.id),
		enabled: autofillEnabled && templateQuery.isSuccess,
	});
	const autofillParameters = getAutofillParameters(
		searchParams,
		userParametersQuery.data ? userParametersQuery.data : [],
	);

	const autoCreationStartedRef = useRef(false);
	const automateWorkspaceCreation = useEffectEvent(async () => {
		if (autoCreationStartedRef.current || !organizationId) {
			return;
		}

		try {
			autoCreationStartedRef.current = true;
			const newWorkspace = await autoCreateWorkspaceMutation.mutateAsync({
				organizationId,
				templateName,
				buildParameters: autofillParameters,
				workspaceName: defaultName ?? generateWorkspaceName(),
				templateVersionId: realizedVersionId,
				match: searchParams.get("match"),
			});

			onCreateWorkspace(newWorkspace);
		} catch {
			setMode("form");
		}
	});

	const hasAllRequiredExternalAuth = Boolean(
		!isLoadingExternalAuth &&
			externalAuth?.every((auth) => auth.optional || auth.authenticated),
	);

	let autoCreateReady =
		mode === "auto" &&
		(!autofillEnabled || userParametersQuery.isSuccess) &&
		hasAllRequiredExternalAuth;

	// `mode=auto` was set, but a prerequisite has failed, and so auto-mode should be abandoned.
	if (
		mode === "auto" &&
		!isLoadingExternalAuth &&
		!hasAllRequiredExternalAuth
	) {
		// Prevent suddenly resuming auto-mode if the user connects to all of the required
		// external auth providers.
		setMode("form");
		// Ensure this is always false, so that we don't ever let `automateWorkspaceCreation`
		// fire when we're trying to disable it.
		autoCreateReady = false;
		// Show an error message to explain _why_ the workspace was not created automatically.
		const subject =
			externalAuth?.length === 1
				? "an external authentication provider that is"
				: "external authentication providers that are";
		setAutoCreateError({
			message: `This template requires ${subject} not connected.`,
			detail:
				"Auto-creation has been disabled. Please connect all required external authentication providers before continuing.",
		});
	}

	useEffect(() => {
		if (autoCreateReady) {
			void automateWorkspaceCreation();
		}
	}, [automateWorkspaceCreation, autoCreateReady]);

	return (
		<>
			<Helmet>
				<title>{pageTitle(title)}</title>
			</Helmet>
			{isLoadingFormData || isLoadingExternalAuth || autoCreateReady ? (
				<Loader />
			) : (
				<CreateWorkspacePageViewExperimental
					mode={mode}
					defaultName={defaultName}
					disabledParams={disabledParams}
					defaultOwner={me}
					autofillParameters={autofillParameters}
					error={
						createWorkspaceMutation.error ||
						autoCreateError ||
						loadFormDataError ||
						autoCreateWorkspaceMutation.error
					}
					resetMutation={createWorkspaceMutation.reset}
					template={templateQuery.data!}
					versionId={realizedVersionId}
					externalAuth={externalAuth ?? []}
					externalAuthPollingState={externalAuthPollingState}
					startPollingExternalAuth={startPollingExternalAuth}
					hasAllRequiredExternalAuth={hasAllRequiredExternalAuth}
					permissions={permissionsQuery.data as WorkspacePermissions}
					parameters={realizedParameters as TemplateVersionParameter[]}
					presets={templateVersionPresetsQuery.data ?? []}
					creatingWorkspace={createWorkspaceMutation.isLoading}
					onCancel={() => {
						navigate(-1);
					}}
					onSubmit={async (request, owner) => {
						if (realizedVersionId) {
							request = {
								...request,
								template_id: undefined,
								template_version_id: realizedVersionId,
							};
						}

						const workspace = await createWorkspaceMutation.mutateAsync({
							...request,
							userId: owner.id,
						});
						onCreateWorkspace(workspace);
					}}
				/>
			)}
		</>
	);
};

const useExternalAuth = (versionId: string | undefined) => {
	const [externalAuthPollingState, setExternalAuthPollingState] =
		useState<ExternalAuthPollingState>("idle");

	const startPollingExternalAuth = useCallback(() => {
		setExternalAuthPollingState("polling");
	}, []);

	const { data: externalAuth, isLoading: isLoadingExternalAuth } = useQuery(
		versionId
			? {
					...templateVersionExternalAuth(versionId),
					refetchInterval:
						externalAuthPollingState === "polling" ? 1000 : false,
				}
			: { enabled: false },
	);

	const allSignedIn = externalAuth?.every((it) => it.authenticated);

	useEffect(() => {
		if (allSignedIn) {
			setExternalAuthPollingState("idle");
			return;
		}

		if (externalAuthPollingState !== "polling") {
			return;
		}

		// Poll for a maximum of one minute
		const quitPolling = setTimeout(
			() => setExternalAuthPollingState("abandoned"),
			60_000,
		);
		return () => {
			clearTimeout(quitPolling);
		};
	}, [externalAuthPollingState, allSignedIn]);

	return {
		startPollingExternalAuth,
		externalAuth,
		externalAuthPollingState,
		isLoadingExternalAuth,
	};
};

const getAutofillParameters = (
	urlSearchParams: URLSearchParams,
	userParameters: UserParameter[],
): AutofillBuildParameter[] => {
	const userParamMap = userParameters.reduce((acc, param) => {
		acc.set(param.name, param);
		return acc;
	}, new Map<string, UserParameter>());

	const buildValues: AutofillBuildParameter[] = Array.from(
		urlSearchParams.keys(),
	)
		.filter((key) => key.startsWith("param."))
		.map((key) => {
			const name = key.replace("param.", "");
			const value = urlSearchParams.get(key) ?? "";
			// URL should take precedence over user parameters
			userParamMap.delete(name);
			return { name, value, source: "url" };
		});

	for (const param of userParamMap.values()) {
		buildValues.push({
			name: param.name,
			value: param.value,
			source: "user_history",
		});
	}
	return buildValues;
};

export default CreateWorkspacePageExperimental;

function getWorkspaceMode(params: URLSearchParams): CreateWorkspaceMode {
	const paramMode = params.get("mode");
	if (createWorkspaceModes.includes(paramMode as CreateWorkspaceMode)) {
		return paramMode as CreateWorkspaceMode;
	}

	return "form";
}
