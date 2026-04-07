import { CircleHelp } from "lucide-react";
import type { FC } from "react";
import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { API } from "#/api/api";
import { DetailedError } from "#/api/errors";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	PreviewParameter,
	WorkspaceBuildParameter,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Link } from "#/components/Link/Link";
import { Loader } from "#/components/Loader/Loader";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { getInitialParameterValues } from "#/modules/workspaces/DynamicParameter/DynamicParameter";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import type { AutofillBuildParameter } from "#/utils/richParameters";
import { useWorkspaceSettings } from "../useWorkspaceSettings";
import { WorkspaceParametersPageViewExperimental } from "./WorkspaceParametersPageViewExperimental";

const WorkspaceParametersPageExperimental: FC = () => {
	const { permissions, workspace } = useWorkspaceSettings();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const templateVersionId = searchParams.get("templateVersionId") ?? undefined;

	const [confirmingRestart, setConfirmingRestart] = useState<{
		open: boolean;
		buildParameters?: WorkspaceBuildParameter[];
	}>({ open: false });

	// autofill the form with the workspace build parameters from the latest build
	const {
		data: latestBuildParameters,
		isLoading: latestBuildParametersLoading,
	} = useQuery({
		queryKey: ["workspaceBuilds", workspace.latest_build.id, "parameters"],
		queryFn: () => API.getWorkspaceBuildParameters(workspace.latest_build.id),
	});

	const [latestResponse, setLatestResponse] =
		useState<DynamicParametersResponse | null>(null);
	const wsResponseId = useRef<number>(-1);
	const ws = useRef<WebSocket | null>(null);
	const [wsError, setWsError] = useState<Error | null>(null);
	const initialParamsSentRef = useRef(false);

	const autofillParameters: AutofillBuildParameter[] =
		latestBuildParameters?.map((p) => ({
			...p,
			source: "active_build",
		})) ?? [];

	const sendMessage = useEffectEvent((formValues: Record<string, string>) => {
		const request: DynamicParametersRequest = {
			id: wsResponseId.current + 1,
			owner_id: workspace.owner_id,
			inputs: formValues,
		};
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(request));
			wsResponseId.current = wsResponseId.current + 1;
		}
	});

	// Sends the user's build parameter values to the WebSocket so the
	// backend evaluates dynamic expressions against real values instead
	// of template defaults. Bails when the REST build parameters
	// haven't loaded yet; the retrigger effect below covers that case.
	const sendInitialParameters = useEffectEvent(
		(parameters: PreviewParameter[]) => {
			if (initialParamsSentRef.current) return;
			if (parameters.length === 0) return;
			if (!latestBuildParameters) return;

			const inputs: Record<string, string> = {};
			for (const p of getInitialParameterValues(
				parameters,
				autofillParameters,
			)) {
				if (p.name && p.value) {
					inputs[p.name] = p.value;
				}
			}
			if (Object.keys(inputs).length === 0) return;

			sendMessage(inputs);
			initialParamsSentRef.current = true;
		},
	);

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		// Skip stale responses. If we've already sent a newer request,
		// this response contains outdated parameter values that would
		// overwrite the user's more recent input.
		if (response.id < wsResponseId.current) {
			return;
		}

		// Send initial params before storing the response so the
		// stale-response guard above filters the defaults on the
		// next WS message.
		if (!initialParamsSentRef.current && response.parameters?.length > 0) {
			sendInitialParameters([...response.parameters]);
		}

		setLatestResponse(response);
	});

	// When the WS first message arrives before the REST build
	// parameters have loaded, sendInitialParameters bails. This
	// effect retriggers the send once both sources are available.
	useEffect(() => {
		if (initialParamsSentRef.current) return;
		if (!latestResponse?.parameters?.length) return;
		if (!latestBuildParameters) return;

		sendInitialParameters([...latestResponse.parameters]);
	}, [latestResponse, latestBuildParameters]);

	useEffect(() => {
		if (!templateVersionId && !workspace.latest_build.template_version_id)
			return;

		const socket = API.templateVersionDynamicParameters(
			templateVersionId ?? workspace.latest_build.template_version_id,
			workspace.owner_id,
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
	}, [
		templateVersionId,
		workspace.latest_build.template_version_id,
		workspace.owner_id,
	]);

	const startWithParameters = useMutation({
		mutationFn: (buildParameters: WorkspaceBuildParameter[]) =>
			API.postWorkspaceBuild(workspace.id, {
				transition: "start",
				template_version_id: templateVersionId,
				rich_parameter_values: buildParameters,
				reason: "dashboard",
			}),
		onSuccess: () => {
			navigate(`/@${workspace.owner_name}/${workspace.name}`);
		},
	});

	const restartWithParameters = useMutation({
		mutationFn: async (buildParameters: WorkspaceBuildParameter[]) => {
			const stopBuild = await API.stopWorkspace(workspace.id);
			const awaitedStopBuild = await API.waitForBuild(stopBuild);

			// If the restart is canceled halfway through, make sure we bail
			if (awaitedStopBuild?.status === "canceled") {
				return;
			}

			return API.postWorkspaceBuild(workspace.id, {
				transition: "start",
				template_version_id: templateVersionId,
				rich_parameter_values: buildParameters,
				reason: "dashboard",
			});
		},
		onSuccess: () => {
			navigate(`/@${workspace.owner_name}/${workspace.name}`);
		},
	});

	const canChangeVersions = Boolean(permissions?.updateWorkspaceVersion);

	const handleSubmit = (values: {
		rich_parameter_values: WorkspaceBuildParameter[];
	}) => {
		if (!latestResponse?.parameters) {
			return;
		}

		// Only submit mutable parameters
		const onlyMutableValues = latestResponse.parameters
			.filter((p) => p.mutable)
			.map((p) => {
				const value = values.rich_parameter_values.find(
					(v) => v.name === p.name,
				);
				if (!value) {
					throw new Error(`Missing value for parameter ${p.name}`);
				}
				return value;
			});

		// We only enable the button to navigate to this page if the workspace can
		// accept new jobs, but if the workspace is in any pending state (user
		// manually loaded the page or workspace state changed after load) then we
		// could still submit a build that will fail.
		if (workspace.latest_build.status === "running") {
			setConfirmingRestart({ open: true, buildParameters: onlyMutableValues });
		} else {
			startWithParameters.mutate(onlyMutableValues);
		}
	};

	const sortedParams = useMemo(() => {
		if (!latestResponse?.parameters) {
			return [];
		}
		return [...latestResponse.parameters].sort((a, b) => a.order - b.order);
	}, [latestResponse?.parameters]);

	const error =
		wsError || startWithParameters.error || restartWithParameters.error;

	// Keep the loader visible until a WS response reflecting
	// the user's actual build parameter values arrives (id >= 0).
	// Without this, the initial id -1 response (template defaults)
	// can overwrite the correct autofill values from the previous
	// build before the form has a chance to render them.
	// We check latestBuildParameters instead of initialParamsSentRef
	// because the ref doesn't trigger re-renders.
	const awaitingUserValues =
		latestResponse !== null &&
		latestResponse.id < 0 &&
		latestBuildParameters !== undefined &&
		!wsError;

	if (
		latestBuildParametersLoading ||
		(!latestResponse && !wsError) ||
		(ws.current && ws.current.readyState === WebSocket.CONNECTING) ||
		awaitingUserValues
	) {
		return <Loader />;
	}

	let submitLabel = "Update and start";
	if (restartWithParameters.isPending) {
		submitLabel = "Stopping workspace";
	} else if (startWithParameters.isPending) {
		submitLabel = "Starting workspace";
	} else if (workspace.latest_build.status === "running") {
		submitLabel = "Update and restart";
	}

	return (
		<div className="flex flex-col gap-6 max-w-screen-md">
			<title>{pageTitle(workspace.name, "Parameters")}</title>

			<header className="flex flex-col items-start gap-2">
				<span className="flex flex-row items-center gap-2 justify-between w-full">
					<span className="flex flex-row items-center gap-2">
						<h1 className="text-3xl m-0">Workspace parameters</h1>
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<CircleHelp className="size-icon-xs text-content-secondary" />
								</TooltipTrigger>
								<TooltipContent className="max-w-xs text-sm">
									Dynamic Parameters enhances Coder's existing parameter system
									with real-time validation, conditional parameter behavior, and
									richer input types.
									<br />
									<Link
										href={docs(
											"/admin/templates/extending-templates/dynamic-parameters",
										)}
									>
										View docs
									</Link>
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					</span>
				</span>
			</header>

			{Boolean(error) && <ErrorAlert error={error} />}

			{sortedParams.length > 0 ? (
				<WorkspaceParametersPageViewExperimental
					templateVersionId={templateVersionId}
					workspace={workspace}
					autofillParameters={autofillParameters}
					canChangeVersions={canChangeVersions}
					parameters={sortedParams}
					diagnostics={latestResponse?.diagnostics ?? []}
					initialParamsAcknowledged={
						!initialParamsSentRef.current ||
						(latestResponse !== null && latestResponse.id >= 0)
					}
					isSubmitting={
						startWithParameters.isPending || restartWithParameters.isPending
					}
					submitLabel={submitLabel}
					onSubmit={handleSubmit}
					onCancel={() =>
						navigate(`/@${workspace.owner_name}/${workspace.name}`)
					}
					sendMessage={sendMessage}
				/>
			) : (
				<EmptyState
					className="border border-border border-solid rounded-md"
					message="This workspace has no parameters"
					cta={
						<Link
							href={docs(
								"/admin/templates/extending-templates/dynamic-parameters",
							)}
						>
							Learn more about parameters
						</Link>
					}
				/>
			)}

			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={confirmingRestart.open}
				onConfirm={() => {
					restartWithParameters.mutate(confirmingRestart.buildParameters ?? []);
					setConfirmingRestart({ open: false });
				}}
				onClose={() => setConfirmingRestart({ open: false })}
				title="Restart your workspace?"
				confirmText="Restart"
				description={
					<>
						Restarting your workspace will stop all running processes and{" "}
						<strong>delete non-persistent data</strong>.
					</>
				}
			/>
		</div>
	);
};

export default WorkspaceParametersPageExperimental;
