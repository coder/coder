import { API } from "api/api";
import { getErrorMessage } from "api/errors";
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
import {
	WorkspaceUpdateDialogs,
	useWorkspaceUpdate,
} from "modules/workspaces/WorkspaceUpdateDialogs";
import type { WorkspacePermissions } from "modules/workspaces/permissions";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { pageTitle } from "utils/page";
import { Workspace } from "./Workspace";

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
	const { mutate: mutateRestartWorkspace, isPending: isRestarting } =
		useMutation({
			mutationFn: API.restartWorkspace,
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
				isUpdating={workspaceUpdate.isUpdating}
				isRestarting={isRestarting}
				workspace={workspace}
				latestVersion={latestVersion}
				template={template}
				buildLogs={buildLogs}
				timings={timingsQuery.data}
				handleStart={(buildParameters) => {
					startWorkspaceMutation.mutate({ buildParameters });
				}}
				handleStop={() => {
					stopWorkspaceMutation.mutate({});
				}}
				handleRestart={(buildParameters) => {
					setConfirmingRestart({ open: true, buildParameters });
				}}
				handleUpdate={workspaceUpdate.update}
				handleCancel={cancelBuildMutation.mutate}
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

			<WorkspaceUpdateDialogs {...workspaceUpdate.dialogs} />
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
