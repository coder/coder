import { API } from "api/api";
import { type ApiErrorResponse, DetailedError } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import {
	templateByName,
	templateVersionExternalAuth,
	templateVersionPresets,
} from "api/queries/templates";
import { autoCreateWorkspace, createWorkspace } from "api/queries/workspaces";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	PreviewParameter,
	Workspace,
} from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "hooks";
import { useEffectEvent } from "hooks/hookPolyfills";
import { getInitialParameterValues } from "modules/workspaces/DynamicParameter/DynamicParameter";
import { generateWorkspaceName } from "modules/workspaces/generateWorkspaceName";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import type { AutofillBuildParameter } from "utils/richParameters";
import { CreateWorkspacePageViewExperimental } from "./CreateWorkspacePageViewExperimental";
import {
	type CreateWorkspacePermissions,
	createWorkspaceChecks,
} from "./permissions";

const createWorkspaceModes = ["form", "auto", "duplicate"] as const;
type CreateWorkspaceMode = (typeof createWorkspaceModes)[number];
type ExternalAuthPollingState = "idle" | "polling" | "abandoned";

const CreateWorkspacePageExperimental: FC = () => {
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
	const [mode, setMode] = useState(() => getWorkspaceMode(searchParams));
	const [autoCreateError, setAutoCreateError] =
		useState<ApiErrorResponse | null>(null);
	const defaultOwner = me;
	const [owner, setOwner] = useState(defaultOwner);

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
		enabled: !!templateQuery.data,
	});
	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: createWorkspaceChecks(templateQuery.data?.organization_id ?? ""),
		}),
		enabled: !!templateQuery.data,
	});
	const realizedVersionId =
		customVersionId ?? templateQuery.data?.active_version_id;

	const autofillParameters = getAutofillParameters(searchParams);

	const sendMessage = useEffectEvent(
		(formValues: Record<string, string>, ownerId?: string) => {
			const request: DynamicParametersRequest = {
				id: wsResponseId.current + 1,
				owner_id: ownerId ?? owner.id,
				inputs: formValues,
			};
			if (ws.current && ws.current.readyState === WebSocket.OPEN) {
				ws.current.send(JSON.stringify(request));
				wsResponseId.current = wsResponseId.current + 1;
			}
		},
	);

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
				if (param.name && param.value) {
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
	}, [realizedVersionId, onMessage, defaultOwner.id]);

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

	let autoCreateReady = mode === "auto" && hasAllRequiredExternalAuth;

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

	const sortedParams = useMemo(() => {
		if (!latestResponse?.parameters) {
			return [];
		}
		return [...latestResponse.parameters].sort((a, b) => a.order - b.order);
	}, [latestResponse?.parameters]);

	return (
		<>
			<Helmet>
				<title>{pageTitle(title)}</title>
			</Helmet>
			{!latestResponse ||
			!templateQuery.data ||
			isLoadingFormData ||
			isLoadingExternalAuth ||
			autoCreateReady ? (
				<Loader />
			) : (
				<CreateWorkspacePageViewExperimental
					mode={mode}
					defaultName={defaultName}
					diagnostics={latestResponse?.diagnostics ?? []}
					disabledParams={disabledParams}
					defaultOwner={defaultOwner}
					owner={owner}
					setOwner={setOwner}
					autofillParameters={autofillParameters}
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
					externalAuth={externalAuth ?? []}
					externalAuthPollingState={externalAuthPollingState}
					startPollingExternalAuth={startPollingExternalAuth}
					hasAllRequiredExternalAuth={hasAllRequiredExternalAuth}
					permissions={permissionsQuery.data as CreateWorkspacePermissions}
					parameters={sortedParams}
					presets={templateVersionPresetsQuery.data ?? []}
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

const useExternalAuth = (versionId: string | undefined) => {
	const [externalAuthPollingState, setExternalAuthPollingState] =
		useState<ExternalAuthPollingState>("idle");

	const startPollingExternalAuth = useCallback(() => {
		setExternalAuthPollingState("polling");
	}, []);

	const { data: externalAuth, isLoading: isLoadingExternalAuth } = useQuery({
		...templateVersionExternalAuth(versionId ?? ""),
		enabled: !!versionId,
		refetchInterval: externalAuthPollingState === "polling" ? 1000 : false,
	});

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

export default CreateWorkspacePageExperimental;

function getWorkspaceMode(params: URLSearchParams): CreateWorkspaceMode {
	const paramMode = params.get("mode");
	if (createWorkspaceModes.includes(paramMode as CreateWorkspaceMode)) {
		return paramMode as CreateWorkspaceMode;
	}

	return "form";
}
