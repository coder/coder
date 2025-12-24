import { API } from "api/api";
import { checkAuthorization } from "api/queries/authCheck";
import type {
	DynamicParametersResponse,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useDynamicParametersWebSocket } from "hooks/useDynamicParametersWebSocket";
import { CircleHelp } from "lucide-react";
import type { FC } from "react";
import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import type { AutofillBuildParameter } from "utils/richParameters";
import {
	type WorkspacePermissions,
	workspaceChecks,
} from "../../../modules/workspaces/permissions";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import { WorkspaceParametersPageViewExperimental } from "./WorkspaceParametersPageViewExperimental";

const WorkspaceParametersPageExperimental: FC = () => {
	const workspace = useWorkspaceSettings();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const templateVersionId = searchParams.get("templateVersionId") ?? undefined;

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
	const initialParamsSentRef = useRef(false);
	// Ref to store current form values for reconnection.
	const currentFormValuesRef = useRef<Record<string, string>>({});

	const autofillParameters: AutofillBuildParameter[] =
		latestBuildParameters?.map((p) => ({
			...p,
			source: "active_build",
		})) ?? [];

	// On page load, sends initial workspace build parameters to the websocket.
	// This ensures the backend has the form's complete initial state,
	// vital for rendering dynamic UI elements dependent on initial parameter values.
	const sendInitialParameters = useEffectEvent(
		(sendFn: (values: Record<string, string>) => void) => {
			if (initialParamsSentRef.current) return;
			if (autofillParameters.length === 0) return;

			const initialParamsToSend: Record<string, string> = {};
			for (const param of autofillParameters) {
				if (param.name && param.value) {
					initialParamsToSend[param.name] = param.value;
				}
			}
			if (Object.keys(initialParamsToSend).length === 0) return;

			sendFn(initialParamsToSend);
			initialParamsSentRef.current = true;
		},
	);

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		if (!initialParamsSentRef.current && response.parameters?.length > 0) {
			sendInitialParameters(sendMessage);
		}

		setLatestResponse(response);
	});

	const onReconnect = useEffectEvent(() => {
		// Resend current form values to restore server-side state after reconnection.
		const currentValues = currentFormValuesRef.current;
		if (Object.keys(currentValues).length > 0) {
			sendMessage(currentValues);
		}
	});

	// Initialize the WebSocket connection with auto-reconnection support.
	const {
		sendMessage,
		status: wsStatus,
		error: wsError,
	} = useDynamicParametersWebSocket({
		templateVersionId:
			templateVersionId ?? workspace.latest_build.template_version_id,
		userId: workspace.owner_id,
		onMessage,
		onReconnect,
	});

	const updateParameters = useMutation({
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

	const checks = workspace ? workspaceChecks(workspace) : {};
	const permissionsQuery = useQuery({
		...checkAuthorization({ checks }),
		enabled: workspace !== undefined,
	});
	const permissions = permissionsQuery.data as WorkspacePermissions | undefined;
	const canChangeVersions = Boolean(permissions?.updateWorkspaceVersion);

	const handleSubmit = (values: {
		rich_parameter_values: WorkspaceBuildParameter[];
	}) => {
		if (!latestResponse || !latestResponse.parameters) {
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

		updateParameters.mutate(onlyMutableValues);
	};

	const sortedParams = useMemo(() => {
		if (!latestResponse?.parameters) {
			return [];
		}
		return [...latestResponse.parameters].sort((a, b) => a.order - b.order);
	}, [latestResponse?.parameters]);

	const error = wsError || updateParameters.error;

	if (
		latestBuildParametersLoading ||
		!latestResponse ||
		wsStatus === "connecting"
	) {
		return <Loader />;
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
					diagnostics={latestResponse.diagnostics}
					isSubmitting={updateParameters.isPending}
					onSubmit={handleSubmit}
					onCancel={() =>
						navigate(`/@${workspace.owner_name}/${workspace.name}`)
					}
					sendMessage={(formValues) => {
						// Store form values for potential reconnection.
						currentFormValuesRef.current = formValues;
						sendMessage(formValues);
					}}
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
		</div>
	);
};

export default WorkspaceParametersPageExperimental;
