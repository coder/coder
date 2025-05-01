import { API, MissingBuildParameters } from "api/api";
import { getErrorMessage } from "api/errors";
import { buildInfo } from "api/queries/buildInfo";
import { deploymentConfig, deploymentSSHConfig } from "api/queries/deployment";
import { templateVersion, templateVersions } from "api/queries/templates";
import { workspaceBuildTimings } from "api/queries/workspaceBuilds";
import {
	activate,
	cancelBuild,
	changeVersion,
	deleteWorkspace,
	startWorkspace,
	stopWorkspace,
	toggleFavorite,
	updateWorkspace,
} from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import {
	ConfirmDialog,
	type ConfirmDialogProps,
} from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError } from "components/GlobalSnackbar/utils";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import { useAuthenticated } from "hooks";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useWorkspaceBuildLogs } from "hooks/useWorkspaceBuildLogs";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { pageTitle } from "utils/page";
import { ChangeVersionDialog } from "./ChangeVersionDialog";
import { UpdateBuildParametersDialog } from "./UpdateBuildParametersDialog";
import { Workspace } from "./Workspace";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";
import type { WorkspacePermissions } from "./permissions";

interface WorkspaceReadyPageProps {
	template: TypesGen.Template;
	workspace: TypesGen.Workspace;
	permissions: WorkspacePermissions;
}

export const WorkspaceReadyPage: FC<WorkspaceReadyPageProps> = ({
	workspace,
	template,
	permissions,
}) => {
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const featureVisibility = useFeatureVisibility();
	if (workspace === undefined) {
		throw Error("Workspace is undefined");
	}

	// Debug mode
	const { data: deploymentValues } = useQuery({
		...deploymentConfig(),
		enabled: permissions.viewDeploymentConfig,
	});

	// Build logs
	const shouldStreamBuildLogs = workspace.latest_build.status !== "running";
	const buildLogs = useWorkspaceBuildLogs(
		workspace.latest_build.id,
		shouldStreamBuildLogs,
	);

	// Restart
	const [confirmingRestart, setConfirmingRestart] = useState<{
		open: boolean;
		buildParameters?: TypesGen.WorkspaceBuildParameter[];
	}>({ open: false });
	const { mutate: mutateRestartWorkspace, isLoading: isRestarting } =
		useMutation({
			mutationFn: API.restartWorkspace,
		});

	// SSH Prefix
	const sshPrefixQuery = useQuery(deploymentSSHConfig());

	// Favicon
	const favicon = getFaviconByStatus(workspace.latest_build);
	const [faviconTheme, setFaviconTheme] = useState<"light" | "dark">("dark");
	useEffect(() => {
		if (typeof window === "undefined" || !window.matchMedia) {
			return;
		}

		const isDark = window.matchMedia("(prefers-color-scheme: dark)");
		// We want the favicon the opposite of the theme.
		setFaviconTheme(isDark.matches ? "light" : "dark");
	}, []);

	// Change version
	const canChangeVersions = permissions.updateTemplate;
	const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
	const changeVersionMutation = useMutation(
		changeVersion(workspace, queryClient),
	);

	// Versions
	const { data: allVersions } = useQuery({
		...templateVersions(workspace.template_id),
		enabled: changeVersionDialogOpen,
	});
	const { data: latestVersion } = useQuery({
		...templateVersion(workspace.template_active_version_id),
		enabled: workspace.outdated,
	});

	// Update workspace
	const [isConfirmingUpdate, setIsConfirmingUpdate] = useState(false);
	const updateWorkspaceMutation = useMutation(
		updateWorkspace(workspace, queryClient),
	);

	// If a user can update the template then they can force a delete
	// (via orphan).
	const canUpdateTemplate = Boolean(permissions.updateTemplate);
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const deleteWorkspaceMutation = useMutation(
		deleteWorkspace(workspace, queryClient),
	);

	// Activate workspace
	const activateWorkspaceMutation = useMutation(
		activate(workspace, queryClient),
	);

	// Stop workspace
	const stopWorkspaceMutation = useMutation(
		stopWorkspace(workspace, queryClient),
	);

	// Start workspace
	const startWorkspaceMutation = useMutation(
		startWorkspace(workspace, queryClient),
	);

	// Toggle workspace favorite
	const toggleFavoriteMutation = useMutation(
		toggleFavorite(workspace, queryClient),
	);

	// Cancel build
	const cancelBuildMutation = useMutation(cancelBuild(workspace, queryClient));

	// Workspace Timings.
	const timingsQuery = useQuery({
		...workspaceBuildTimings(workspace.latest_build.id),

		// Fetch build timings only when the build job is completed.
		enabled: Boolean(workspace.latest_build.job.completed_at),

		// Sometimes, the timings can be fetched before the agent script timings are
		// done or saved in the database so we need to conditionally refetch the
		// timings. To refetch the timings, I found the best way was to compare the
		// expected amount of script timings that run on start, with the current
		// amount of script timings returned in the response.
		refetchInterval: (data) => {
			const expectedScriptTimingsCount = workspace.latest_build.resources
				.flatMap((r) => r.agents)
				.flatMap((a) => a?.scripts ?? [])
				.filter((script) => script.run_on_start).length;
			const currentScriptTimingsCount = data?.agent_script_timings?.length ?? 0;

			return expectedScriptTimingsCount === currentScriptTimingsCount
				? false
				: 1_000;
		},
	});

	const runLastBuild = (
		buildParameters: TypesGen.WorkspaceBuildParameter[] | undefined,
		debug: boolean,
	) => {
		const logLevel = debug ? "debug" : undefined;

		switch (workspace.latest_build.transition) {
			case "start":
				startWorkspaceMutation.mutate({
					logLevel,
					buildParameters,
				});
				break;
			case "stop":
				stopWorkspaceMutation.mutate({ logLevel });
				break;
			case "delete":
				deleteWorkspaceMutation.mutate({ log_level: logLevel });
				break;
		}
	};

	const handleRetry = (
		buildParameters?: TypesGen.WorkspaceBuildParameter[],
	) => {
		runLastBuild(buildParameters, false);
	};

	const handleDebug = (
		buildParameters?: TypesGen.WorkspaceBuildParameter[],
	) => {
		runLastBuild(buildParameters, true);
	};

	return (
		<>
			<Helmet>
				<title>{pageTitle(`${workspace.owner_name}/${workspace.name}`)}</title>
				<link
					rel="alternate icon"
					type="image/png"
					href={`/favicons/${favicon}-${faviconTheme}.png`}
				/>
				<link
					rel="icon"
					type="image/svg+xml"
					href={`/favicons/${favicon}-${faviconTheme}.svg`}
				/>
			</Helmet>

			<Workspace
				permissions={permissions}
				isUpdating={updateWorkspaceMutation.isLoading}
				isRestarting={isRestarting}
				workspace={workspace}
				handleStart={(buildParameters) => {
					startWorkspaceMutation.mutate({ buildParameters });
				}}
				handleStop={() => {
					stopWorkspaceMutation.mutate({});
				}}
				handleDelete={() => {
					setIsConfirmingDelete(true);
				}}
				handleRestart={(buildParameters) => {
					setConfirmingRestart({ open: true, buildParameters });
				}}
				handleUpdate={() => {
					setIsConfirmingUpdate(true);
				}}
				handleCancel={cancelBuildMutation.mutate}
				handleSettings={() => navigate("settings")}
				handleRetry={handleRetry}
				handleDebug={handleDebug}
				canDebugMode={
					deploymentValues?.config.enable_terraform_debug_mode ?? false
				}
				handleChangeVersion={() => {
					setChangeVersionDialogOpen(true);
				}}
				handleDormantActivate={async () => {
					try {
						await activateWorkspaceMutation.mutateAsync();
					} catch (e) {
						const message = getErrorMessage(e, "Error activate workspace.");
						displayError(message);
					}
				}}
				handleToggleFavorite={() => {
					toggleFavoriteMutation.mutate();
				}}
				latestVersion={latestVersion}
				canChangeVersions={canChangeVersions}
				hideSSHButton={featureVisibility.browser_only}
				hideVSCodeDesktopButton={featureVisibility.browser_only}
				buildInfo={buildInfoQuery.data}
				sshPrefix={sshPrefixQuery.data?.hostname_prefix}
				template={template}
				buildLogs={buildLogs}
				timings={timingsQuery.data}
			/>

			<WorkspaceDeleteDialog
				workspace={workspace}
				canUpdateTemplate={canUpdateTemplate}
				isOpen={isConfirmingDelete}
				onCancel={() => {
					setIsConfirmingDelete(false);
				}}
				onConfirm={(orphan) => {
					deleteWorkspaceMutation.mutate({ orphan });
					setIsConfirmingDelete(false);
				}}
				workspaceBuildDateStr={dayjs(workspace.created_at).fromNow()}
			/>

			<UpdateBuildParametersDialog
				missedParameters={
					changeVersionMutation.error instanceof MissingBuildParameters
						? changeVersionMutation.error.parameters
						: []
				}
				open={changeVersionMutation.error instanceof MissingBuildParameters}
				onClose={() => {
					changeVersionMutation.reset();
				}}
				onUpdate={(buildParameters) => {
					if (changeVersionMutation.error instanceof MissingBuildParameters) {
						changeVersionMutation.mutate({
							versionId: changeVersionMutation.error.versionId,
							buildParameters,
						});
					}
				}}
			/>

			<UpdateBuildParametersDialog
				missedParameters={
					updateWorkspaceMutation.error instanceof MissingBuildParameters
						? updateWorkspaceMutation.error.parameters
						: []
				}
				open={updateWorkspaceMutation.error instanceof MissingBuildParameters}
				onClose={() => {
					updateWorkspaceMutation.reset();
				}}
				onUpdate={(buildParameters) => {
					if (updateWorkspaceMutation.error instanceof MissingBuildParameters) {
						updateWorkspaceMutation.mutate(buildParameters);
					}
				}}
			/>

			<ChangeVersionDialog
				templateVersions={allVersions?.reverse()}
				template={template}
				defaultTemplateVersion={allVersions?.find(
					(v) => workspace.latest_build.template_version_id === v.id,
				)}
				open={changeVersionDialogOpen}
				onClose={() => {
					setChangeVersionDialogOpen(false);
				}}
				onConfirm={(templateVersion) => {
					setChangeVersionDialogOpen(false);
					changeVersionMutation.mutate({ versionId: templateVersion.id });
				}}
			/>

			<WarningDialog
				open={isConfirmingUpdate}
				onConfirm={() => {
					updateWorkspaceMutation.mutate(undefined);
					setIsConfirmingUpdate(false);
				}}
				onClose={() => setIsConfirmingUpdate(false)}
				title="Update workspace?"
				confirmText="Update"
				description={
					<Stack>
						<p>
							Updating your workspace will start the workspace on the latest
							template version. This can{" "}
							<strong>delete non-persistent data</strong>.
						</p>
						{latestVersion?.message && (
							<MemoizedInlineMarkdown allowedElements={["ol", "ul", "li"]}>
								{latestVersion.message}
							</MemoizedInlineMarkdown>
						)}
					</Stack>
				}
			/>

			<WarningDialog
				open={confirmingRestart.open}
				onConfirm={() => {
					mutateRestartWorkspace({
						workspace,
						buildParameters: confirmingRestart.buildParameters,
					});
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
		</>
	);
};

const WarningDialog: FC<
	Pick<
		ConfirmDialogProps,
		"open" | "onClose" | "title" | "confirmText" | "description" | "onConfirm"
	>
> = (props) => {
	return <ConfirmDialog type="info" hideCancel={false} {...props} />;
};

// You can see the favicon designs here: https://www.figma.com/file/YIGBkXUcnRGz2ZKNmLaJQf/Coder-v2-Design?node-id=560%3A620
type FaviconType =
	| "favicon"
	| "favicon-success"
	| "favicon-error"
	| "favicon-warning"
	| "favicon-running";

const getFaviconByStatus = (build: TypesGen.WorkspaceBuild): FaviconType => {
	switch (build.status) {
		case undefined:
			return "favicon";
		case "running":
			return "favicon-success";
		case "starting":
			return "favicon-running";
		case "stopping":
			return "favicon-running";
		case "stopped":
			return "favicon";
		case "deleting":
			return "favicon";
		case "deleted":
			return "favicon";
		case "canceling":
			return "favicon-warning";
		case "canceled":
			return "favicon";
		case "failed":
			return "favicon-error";
		case "pending":
			return "favicon";
	}
};
