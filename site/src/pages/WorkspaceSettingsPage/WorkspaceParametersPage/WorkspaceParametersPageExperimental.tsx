import { API } from "api/api";
import { DetailedError } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import { useEffectEvent } from "hooks/hookPolyfills";
import type { FC } from "react";
import {
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery } from "react-query";
import { useNavigate } from "react-router-dom";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import {
	type WorkspacePermissions,
	workspaceChecks,
} from "../../../modules/workspaces/permissions";
import { ExperimentalFormContext } from "../../CreateWorkspacePage/ExperimentalFormContext";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import { WorkspaceParametersPageViewExperimental } from "./WorkspaceParametersPageViewExperimental";

const WorkspaceParametersPageExperimental: FC = () => {
	const workspace = useWorkspaceSettings();
	const navigate = useNavigate();
	const experimentalFormContext = useContext(ExperimentalFormContext);

	const [latestResponse, setLatestResponse] =
		useState<DynamicParametersResponse | null>(null);
	const wsResponseId = useRef<number>(-1);
	const ws = useRef<WebSocket | null>(null);
	const [wsError, setWsError] = useState<Error | null>(null);

	const sendMessage = useCallback((formValues: Record<string, string>) => {
		const request: DynamicParametersRequest = {
			id: wsResponseId.current + 1,
			inputs: formValues,
		};
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(request));
			wsResponseId.current = wsResponseId.current + 1;
		}
	}, []);

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		setLatestResponse(response);
	});

	useEffect(() => {
		if (!workspace.latest_build.template_version_id) return;

		const socket = API.templateVersionDynamicParameters(
			workspace.owner_id,
			workspace.latest_build.template_version_id,
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
		workspace.owner_id,
		workspace.latest_build.template_version_id,
		onMessage,
	]);

	const updateParameters = useMutation({
		mutationFn: (buildParameters: WorkspaceBuildParameter[]) =>
			API.postWorkspaceBuild(workspace.id, {
				transition: "start",
				rich_parameter_values: buildParameters,
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
		!latestResponse ||
		(ws.current && ws.current.readyState === WebSocket.CONNECTING)
	) {
		return <Loader />;
	}

	return (
		<div className="flex flex-col gap-6 max-w-screen-md mx-auto">
			<Helmet>
				<title>{pageTitle(workspace.name, "Parameters")}</title>
			</Helmet>

			<header className="flex flex-col items-start gap-2">
				<span className="flex flex-row items-center gap-2">
					<h1 className="text-3xl m-0">Workspace parameters</h1>
					<FeatureStageBadge contentType={"beta"} />
				</span>
				{experimentalFormContext && (
					<Button
						size="sm"
						variant="outline"
						onClick={experimentalFormContext.toggleOptedOut}
					>
						Go back to the classic workspace parameters view
					</Button>
				)}
			</header>

			{Boolean(error) && <ErrorAlert error={error} />}

			{sortedParams.length > 0 ? (
				<WorkspaceParametersPageViewExperimental
					workspace={workspace}
					canChangeVersions={canChangeVersions}
					parameters={sortedParams}
					diagnostics={latestResponse.diagnostics}
					isSubmitting={updateParameters.isLoading}
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
							href={docs("/admin/templates/extending-templates/parameters")}
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
