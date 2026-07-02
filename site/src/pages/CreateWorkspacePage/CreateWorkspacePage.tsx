import {
	type FC,
	useCallback,
	useEffect,
	useEffectEvent,
	useMemo,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { API } from "#/api/api";
import { type ApiErrorResponse, DetailedError } from "#/api/errors";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	templateByName,
	templateVersion,
	templateVersionPresets,
} from "#/api/queries/templates";
import { autoCreateWorkspace, createWorkspace } from "#/api/queries/workspaces";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	MinimalUser,
	PreviewParameter,
	Workspace,
} from "#/api/typesGenerated";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useExternalAuth } from "#/hooks/useExternalAuth";
import { getInitialParameterValues } from "#/modules/workspaces/DynamicParameter/DynamicParameter";
import { generateWorkspaceName } from "#/modules/workspaces/generateWorkspaceName";
import { pageTitle } from "#/utils/page";
import type { AutofillBuildParameter } from "#/utils/richParameters";
import { AutoCreateConsentDialog } from "./AutoCreateConsentDialog";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";
import {
	type CreateWorkspacePermissions,
	createWorkspaceChecks,
} from "./permissions";

const createWorkspaceModes = ["form", "auto", "duplicate"] as const;
export type CreateWorkspaceMode = (typeof createWorkspaceModes)[number];

const CreateWorkspacePage: FC = () => {
	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const { user: me } = useAuthenticated();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();

	const [latestResponse, setLatestResponse] =
		useState<DynamicParametersResponse | null>(null);
	const wsResponseId = useRef<number>(-1);
	const ws = useRef<WebSocket | null>(null);
	const [wsError, setWsError] = useState<Error | null>(null);
	const initialParamsSentRef = useRef(false);

	const customVersionId = searchParams.get("version") ?? undefined;
	const defaultName = searchParams.get("name");
	const disabledParams = searchParams.get("disable_params")?.split(",");
	const presetName = searchParams.get("preset") || undefined;
	const [mode, setMode] = useState(() => getWorkspaceMode(searchParams));
	const [autoCreateConsented, setAutoCreateConsented] = useState(false);
	const [autoCreateError, setAutoCreateError] =
		useState<ApiErrorResponse | null>(null);
	const defaultOwner: MinimalUser = me;
	const [owner, setOwner] = useState<MinimalUser>(defaultOwner);

	const queryClient = useQueryClient();
	const autoCreateWorkspaceMutation = useMutation(
		autoCreateWorkspace(queryClient),
	);
	const createWorkspaceMutation = useMutation(createWorkspace(queryClient));

	const templateQuery = useQuery(
		templateByName(organizationName, templateName),
	);
	const realizedVersionId =
		customVersionId ?? templateQuery.data?.active_version_id;

	const templateVersionPresetsQuery = useQuery({
		...templateVersionPresets(realizedVersionId ?? ""),
		enabled: realizedVersionId !== undefined,
	});
	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: createWorkspaceChecks(
				templateQuery.data?.organization_id ?? "",
				templateQuery.data?.id,
			),
		}),
		enabled: Boolean(templateQuery.data),
	});

	const templateVersionQuery = useQuery({
		...templateVersion(realizedVersionId ?? ""),
		enabled: realizedVersionId !== undefined,
	});

	const effectivePresetName = mode === "duplicate" ? undefined : presetName;

	const presets = templateVersionPresetsQuery.data ?? [];

	const urlPresetResult = useMemo(() => {
		if (!effectivePresetName) return { preset: undefined, error: undefined };

		if (templateVersionPresetsQuery.isError) {
			return {
				preset: undefined,
				error: `Failed to load presets: ${templateVersionPresetsQuery.error?.message ?? "unknown error"}. Please try refreshing the page.`,
			};
		}

		if (!templateVersionPresetsQuery.isSuccess) {
			return { preset: undefined, error: undefined }; // Still loading
		}

		const found = presets.find((p) => p.Name === effectivePresetName);
		if (!found) {
			const versionLabel = templateVersionQuery.data?.name ?? realizedVersionId;
			return {
				preset: undefined,
				error: `Preset "${effectivePresetName}" not found on template version "${versionLabel}". Check that the preset name matches exactly (names are case-sensitive).`,
			};
		}
		return { preset: found, error: undefined };
	}, [
		effectivePresetName,
		presets,
		templateVersionPresetsQuery.isSuccess,
		templateVersionPresetsQuery.isError,
		templateVersionPresetsQuery.error,
		realizedVersionId,
		templateVersionQuery.data?.name,
	]);

	const urlAutofillParameters = useMemo(
		() => getAutofillParameters(searchParams),
		[searchParams],
	);
	const autofillParameters = useMemo(() => {
		if (!urlPresetResult.preset) return urlAutofillParameters;

		const presetParams: AutofillBuildParameter[] =
			urlPresetResult.preset.Parameters.map((p) => ({
				name: p.Name,
				value: p.Value,
				source: "url" as const,
			}));

		return presetParams;
	}, [urlPresetResult.preset, urlAutofillParameters]);

	const hasIgnoredUrlParams =
		urlAutofillParameters.length > 0 && urlPresetResult.preset !== undefined;

	const sendMessage = (
		formValues: Record<string, string>,
		ownerId?: string,
	) => {
		const request: DynamicParametersRequest = {
			id: wsResponseId.current + 1,
			owner_id: ownerId ?? owner.id,
			inputs: formValues,
		};
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(request));
			wsResponseId.current = wsResponseId.current + 1;
		}
	};

	// On page load, sends all initial parameter values to the websocket
	// (including defaults and autofilled from the url)
	// This ensures the backend has the complete initial state of the form,
	// which is vital for correctly rendering dynamic UI elements where parameter visibility
	// or options might depend on the initial values of other parameters.
	const sendInitialParameters = useEffectEvent(
		(parameters: PreviewParameter[]) => {
			if (initialParamsSentRef.current) return;
			if (parameters.length === 0) return;

			const initialFormValues = getInitialParameterValues(
				parameters,
				autofillParameters,
			);
			if (initialFormValues.length === 0) return;

			const initialParamsToSend: Record<string, string> = {};
			for (const param of initialFormValues) {
				if (param.name && param.value !== undefined) {
					initialParamsToSend[param.name] = param.value;
				}
			}

			if (Object.keys(initialParamsToSend).length === 0) return;

			sendMessage(initialParamsToSend);
			initialParamsSentRef.current = true;
		},
	);

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		if (!initialParamsSentRef.current && response.parameters?.length > 0) {
			sendInitialParameters([...response.parameters]);
		}

		setLatestResponse(response);
	});

	// Initialize the WebSocket connection when there is a valid template version ID
	useEffect(() => {
		if (!realizedVersionId) return;

		const socket = API.templateVersionDynamicParameters(
			realizedVersionId,
			defaultOwner.id,
			{
				onMessage,
				onError: (error) => {
					if (ws.current === socket) {
						setWsError(error);
					}
				},
				onClose: () => {
					if (ws.current === socket) {
						setWsError(
							new DetailedError(
								"Websocket connection for dynamic parameters unexpectedly closed.",
								"Refresh the page to reset the form.",
							),
						);
					}
				},
			},
		);

		ws.current = socket;

		return () => {
			socket.close();
		};
	}, [realizedVersionId, defaultOwner.id]);

	const organizationId = templateQuery.data?.organization_id;

	const {
		externalAuth,
		externalAuthPollingState,
		startPollingExternalAuth,
		isLoadingExternalAuth,
	} = useExternalAuth(realizedVersionId);

	const isLoadingFormData =
		ws.current?.readyState === WebSocket.CONNECTING ||
		templateQuery.isLoading ||
		permissionsQuery.isLoading;
	const loadFormDataError = templateQuery.error ?? permissionsQuery.error;

	const title = autoCreateWorkspaceMutation.isPending
		? "Creating workspace..."
		: "Create workspace";

	const onCreateWorkspace = useCallback(
		(workspace: Workspace) => {
			navigate(`/@${workspace.owner_name}/${workspace.name}`);
		},
		[navigate],
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
				buildParameters: urlPresetResult.preset ? [] : autofillParameters,
				workspaceName: defaultName ?? generateWorkspaceName(),
				templateVersionId: realizedVersionId,
				match: searchParams.get("match"),
				templateVersionPresetId: urlPresetResult.preset?.ID,
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

	const presetResolved =
		!effectivePresetName ||
		(templateVersionPresetsQuery.isSuccess &&
			urlPresetResult.preset !== undefined);

	let autoCreateReady =
		mode === "auto" &&
		hasAllRequiredExternalAuth &&
		autoCreateConsented &&
		presetResolved;

	const showAutoCreateConsent =
		mode === "auto" &&
		!autoCreateConsented &&
		!autoCreateError &&
		presetResolved;

	// `mode=auto` was set, but a prerequisite has failed, and so auto-mode should be abandoned.
	if (
		Boolean(realizedVersionId) &&
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

	if (
		mode === "auto" &&
		hasAllRequiredExternalAuth &&
		effectivePresetName &&
		((templateVersionPresetsQuery.isSuccess && !urlPresetResult.preset) ||
			templateVersionPresetsQuery.isError)
	) {
		setMode("form");
		autoCreateReady = false;
		setAutoCreateError({
			message: "Auto-creation has been disabled.",
			detail:
				urlPresetResult.error ??
				"The requested preset could not be resolved. Please check the preset value before continuing.",
		});
	}

	useEffect(() => {
		if (autoCreateReady) {
			void automateWorkspaceCreation();
		}
	}, [autoCreateReady]);

	const sortedParams = useMemo(() => {
		if (!latestResponse?.parameters) {
			return [];
		}
		return [...latestResponse.parameters].sort((a, b) => a.order - b.order);
	}, [latestResponse?.parameters]);

	const shouldShowLoader =
		!templateQuery.data ||
		isLoadingFormData ||
		isLoadingExternalAuth ||
		autoCreateReady ||
		(!latestResponse && !wsError) ||
		(effectivePresetName &&
			!templateVersionPresetsQuery.isSuccess &&
			!templateVersionPresetsQuery.isError);

	return (
		<>
			<title>{pageTitle(title)}</title>

			<AutoCreateConsentDialog
				open={showAutoCreateConsent}
				presetName={effectivePresetName}
				autofillParameters={autofillParameters}
				onConfirm={() => setAutoCreateConsented(true)}
				onDeny={() => setMode("form")}
			/>

			{shouldShowLoader ? (
				<Loader />
			) : (
				<CreateWorkspacePageView
					mode={mode}
					defaultName={defaultName}
					diagnostics={latestResponse?.diagnostics ?? []}
					disabledParams={disabledParams}
					defaultOwner={defaultOwner}
					owner={owner}
					setOwner={setOwner}
					autofillParameters={autofillParameters}
					canUpdateTemplate={permissionsQuery.data?.canUpdateTemplate}
					error={
						wsError ||
						createWorkspaceMutation.error ||
						autoCreateError ||
						loadFormDataError ||
						autoCreateWorkspaceMutation.error
					}
					resetMutation={createWorkspaceMutation.reset}
					template={templateQuery.data}
					versionId={realizedVersionId}
					versionName={templateVersionQuery.data?.name}
					externalAuth={externalAuth ?? []}
					externalAuthPollingState={externalAuthPollingState}
					startPollingExternalAuth={startPollingExternalAuth}
					hasAllRequiredExternalAuth={hasAllRequiredExternalAuth}
					permissions={permissionsQuery.data as CreateWorkspacePermissions}
					parameters={sortedParams}
					presets={presets}
					urlPreset={urlPresetResult.preset}
					urlPresetError={
						autoCreateError?.detail === urlPresetResult.error
							? undefined
							: urlPresetResult.error
					}
					hasIgnoredUrlParams={hasIgnoredUrlParams}
					creatingWorkspace={createWorkspaceMutation.isPending}
					sendMessage={sendMessage}
					onCancel={() => {
						navigate(-1);
					}}
					onSubmit={async (request, owner) => {
						let workspaceRequest = request;
						if (realizedVersionId) {
							workspaceRequest = {
								...request,
								template_id: undefined,
								template_version_id: realizedVersionId,
							};
						}

						const workspace = await createWorkspaceMutation.mutateAsync({
							...workspaceRequest,
							userId: owner.id,
						});
						onCreateWorkspace(workspace);
					}}
				/>
			)}
		</>
	);
};

const getAutofillParameters = (
	urlSearchParams: URLSearchParams,
): AutofillBuildParameter[] => {
	const buildValues: AutofillBuildParameter[] = Array.from(
		urlSearchParams.keys(),
	)
		.filter((key) => key.startsWith("param."))
		.map((key) => {
			const name = key.replace("param.", "");
			const value = urlSearchParams.get(key) ?? "";
			return { name, value, source: "url" };
		});
	return buildValues;
};

export default CreateWorkspacePage;

function getWorkspaceMode(params: URLSearchParams): CreateWorkspaceMode {
	const paramMode = params.get("mode");
	if (createWorkspaceModes.includes(paramMode as CreateWorkspaceMode)) {
		return paramMode as CreateWorkspaceMode;
	}

	return "form";
}
