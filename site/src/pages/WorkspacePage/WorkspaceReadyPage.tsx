import { API } from "api/api";
import { type ApiError, getErrorMessage, isApiError } from "api/errors";
import { templateVersion } from "api/queries/templates";
import { workspaceBuildTimings } from "api/queries/workspaceBuilds";
import {
	activate,
	cancelBuild,
	deleteWorkspace,
	startWorkspace,
	stopWorkspace,
	toggleFavorite,
} from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import {
	ConfirmDialog,
	type ConfirmDialogProps,
} from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError } from "components/GlobalSnackbar/utils";
import { useWorkspaceBuildLogs } from "hooks/useWorkspaceBuildLogs";
import { EphemeralParametersDialog } from "modules/workspaces/EphemeralParametersDialog/EphemeralParametersDialog";
import { WorkspaceErrorDialog } from "modules/workspaces/ErrorDialog/WorkspaceErrorDialog";
import type { WorkspacePermissions } from "modules/workspaces/permissions";
import { WorkspaceBuildCancelDialog } from "modules/workspaces/WorkspaceBuildCancelDialog/WorkspaceBuildCancelDialog";
import {
	useWorkspaceUpdate,
	WorkspaceUpdateDialogs,
} from "modules/workspaces/WorkspaceUpdateDialogs";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { pageTitle } from "utils/page";
import { Workspace } from "./Workspace";

interface WorkspaceReadyPageProps {
	template: TypesGen.Template;
	workspace: TypesGen.Workspace;
	permissions: WorkspacePermissions;
	sharingDisabled?: boolean;
}

export const WorkspaceReadyPage: FC<WorkspaceReadyPageProps> = ({
	workspace,
	template,
	permissions,
	sharingDisabled,
}) => {
	const queryClient = useQueryClient();

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

	const [workspaceErrorDialog, setWorkspaceErrorDialog] = useState<{
		open: boolean;
		error?: ApiError;
	}>({ open: false });

	const handleError = (error: unknown) => {
		if (isApiError(error) && error.code === "ERR_BAD_REQUEST") {
			setWorkspaceErrorDialog({
				open: true,
				error: error,
			});
		} else {
			displayError(getErrorMessage(error, "Failed to build workspace."));
		}
	};

	const [ephemeralParametersDialog, setEphemeralParametersDialog] = useState<{
		open: boolean;
		action: "start" | "restart";
		buildParameters?: TypesGen.WorkspaceBuildParameter[];
		ephemeralParameters: TypesGen.TemplateVersionParameter[];
	}>({ open: false, action: "start", ephemeralParameters: [] });

	const [isCancelConfirmOpen, setIsCancelConfirmOpen] = useState(false);

	const { mutate: mutateRestartWorkspace, isPending: isRestarting } =
		useMutation({
			mutationFn: API.restartWorkspace,
			onError: (error: unknown) => {
				handleError(error);
			},
		});

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

	// Active version
	const { data: latestVersion } = useQuery({
		...templateVersion(workspace.template_active_version_id),
		enabled: workspace.outdated,
	});

	// Update workspace
	const workspaceUpdate = useWorkspaceUpdate({
		workspace,
		latestVersion,
	});

	// Delete workspace
	const deleteWorkspaceMutation = useMutation({
		...deleteWorkspace(workspace, queryClient),
		onError: (error: unknown) => {
			handleError(error);
		},
	});

	// Activate workspace
	const activateWorkspaceMutation = useMutation({
		...activate(workspace, queryClient),
		onError: (error: unknown) => {
			handleError(error);
		},
	});

	// Stop workspace
	const stopWorkspaceMutation = useMutation({
		...stopWorkspace(workspace, queryClient),
		onError: (error: unknown) => {
			handleError(error);
		},
	});

	// Start workspace
	const startWorkspaceMutation = useMutation({
		...startWorkspace(workspace, queryClient),
		onError: (error: unknown) => {
			handleError(error);
		},
	});

	// Toggle workspace favorite
	const toggleFavoriteMutation = useMutation({
		...toggleFavorite(workspace, queryClient),
		onError: (error: unknown) => {
			handleError(error);
		},
	});

	// Cancel build
	const cancelBuildMutation = useMutation({
		...cancelBuild(workspace, queryClient),
		onError: (error: unknown) => {
			handleError(error);
		},
	});

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
		refetchInterval: ({ state }) => {
			const { data } = state;
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

	const checkEphemeralParameters = async (
		buildParameters?: TypesGen.WorkspaceBuildParameter[],
	) => {
		if (workspace.template_use_classic_parameter_flow) {
			return { hasEphemeral: false, ephemeralParameters: [] };
		}

		try {
			const dynamicParameters = await API.getDynamicParameters(
				workspace.latest_build.template_version_id,
				workspace.owner_id,
				buildParameters || [],
			);

			const ephemeralParameters = dynamicParameters.filter(
				(param) => param.ephemeral,
			);

			return {
				hasEphemeral: ephemeralParameters.length > 0,
				ephemeralParameters,
			};
		} catch (_error) {
			return { hasEphemeral: false, ephemeralParameters: [] };
		}
	};

	const runLastBuild = async (
		buildParameters: TypesGen.WorkspaceBuildParameter[] | undefined,
		debug: boolean,
	) => {
		const logLevel = debug ? "debug" : undefined;

		const { hasEphemeral, ephemeralParameters } =
			await checkEphemeralParameters(buildParameters);
		if (hasEphemeral) {
			setEphemeralParametersDialog({
				open: true,
				action: "start",
				buildParameters,
				ephemeralParameters,
			});
		} else {
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
		}
	};

	const handleRetry = async (
		buildParameters?: TypesGen.WorkspaceBuildParameter[],
	) => {
		await runLastBuild(buildParameters, false);
	};

	const handleDebug = async (
		buildParameters?: TypesGen.WorkspaceBuildParameter[],
	) => {
		await runLastBuild(buildParameters, true);
	};

	return (
		<>
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

			<Workspace
				permissions={permissions}
				isUpdating={workspaceUpdate.isUpdating}
				isRestarting={isRestarting}
				workspace={workspace}
				latestVersion={latestVersion}
				template={template}
				buildLogs={buildLogs}
				timings={timingsQuery.data}
				sharingDisabled={sharingDisabled}
				handleStart={async (buildParameters) => {
					const { hasEphemeral, ephemeralParameters } =
						await checkEphemeralParameters(buildParameters);
					if (hasEphemeral) {
						setEphemeralParametersDialog({
							open: true,
							action: "start",
							buildParameters,
							ephemeralParameters,
						});
					} else {
						startWorkspaceMutation.mutate({ buildParameters });
					}
				}}
				handleStop={() => {
					stopWorkspaceMutation.mutate({});
				}}
				handleRestart={async (buildParameters) => {
					const { hasEphemeral, ephemeralParameters } =
						await checkEphemeralParameters(buildParameters);
					if (hasEphemeral) {
						setEphemeralParametersDialog({
							open: true,
							action: "restart",
							buildParameters,
							ephemeralParameters,
						});
					} else {
						setConfirmingRestart({ open: true, buildParameters });
					}
				}}
				handleUpdate={workspaceUpdate.update}
				handleCancel={() => setIsCancelConfirmOpen(true)}
				handleRetry={handleRetry}
				handleDebug={handleDebug}
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

			<WorkspaceBuildCancelDialog
				open={isCancelConfirmOpen}
				onClose={() => setIsCancelConfirmOpen(false)}
				onConfirm={() => {
					cancelBuildMutation.mutate();
					setIsCancelConfirmOpen(false);
				}}
				workspace={workspace}
			/>

			<EphemeralParametersDialog
				open={ephemeralParametersDialog.open}
				onClose={() =>
					setEphemeralParametersDialog({
						...ephemeralParametersDialog,
						open: false,
					})
				}
				onContinue={() => {
					if (ephemeralParametersDialog.action === "start") {
						startWorkspaceMutation.mutate({
							buildParameters: ephemeralParametersDialog.buildParameters,
						});
					} else {
						setConfirmingRestart({
							open: true,
							buildParameters: ephemeralParametersDialog.buildParameters,
						});
					}
					setEphemeralParametersDialog({
						...ephemeralParametersDialog,
						open: false,
					});
				}}
				ephemeralParameters={ephemeralParametersDialog.ephemeralParameters}
				workspaceOwner={workspace.owner_name}
				workspaceName={workspace.name}
				templateVersionId={workspace.latest_build.template_version_id}
			/>

			<WorkspaceUpdateDialogs {...workspaceUpdate.dialogs} />

			<WorkspaceErrorDialog
				open={workspaceErrorDialog.open}
				error={workspaceErrorDialog.error}
				onClose={() => setWorkspaceErrorDialog({ open: false })}
				showDetail={workspace.template_use_classic_parameter_flow}
				workspaceOwner={workspace.owner_name}
				workspaceName={workspace.name}
				templateVersionId={workspace.latest_build.template_version_id}
				isDeleting={false}
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
