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

	const [currentResponse, setCurrentResponse] =
		useState<DynamicParametersResponse | null>(null);
	const [wsResponseId, setWSResponseId] = useState<number>(-1);
	const ws = useRef<WebSocket | null>(null);
	const [wsError, setWsError] = useState<Error | null>(null);

	const onMessage = useCallback((response: DynamicParametersResponse) => {
		setCurrentResponse((prev) => {
			if (prev?.id === response.id) {
				return prev;
			}
			return response;
		});
	}, []);

	useEffect(() => {
		if (!workspace.latest_build.template_version_id) return;

		const socket = API.templateVersionDynamicParameters(
			workspace.owner_id,
			workspace.latest_build.template_version_id,
			{
				onMessage,
				onError: (error) => {
					setWsError(error);
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

	const sendMessage = useCallback((formValues: Record<string, string>) => {
		setWSResponseId((prevId) => {
			const request: DynamicParametersRequest = {
				id: prevId + 1,
				inputs: formValues,
			};
			if (ws.current && ws.current.readyState === WebSocket.OPEN) {
				ws.current.send(JSON.stringify(request));
				return prevId + 1;
			}
			return prevId;
		});
	}, []);

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
		if (!currentResponse || !currentResponse.parameters) {
			return;
		}

		// Only submit mutable parameters
		const onlyMutableValues = currentResponse.parameters
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
		if (!currentResponse?.parameters) {
			return [];
		}
		return [...currentResponse.parameters].sort((a, b) => a.order - b.order);
	}, [currentResponse?.parameters]);

	const error = wsError || updateParameters.error;

	if (
		!currentResponse ||
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
					diagnostics={currentResponse.diagnostics}
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
