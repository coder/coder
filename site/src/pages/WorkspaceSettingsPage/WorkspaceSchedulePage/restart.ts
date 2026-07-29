import type { WorkspaceStatus } from "#/api/typesGenerated";

interface ShouldConfirmAutostopRestartOptions {
	autostopChanged: boolean;
	autostopEnabled: boolean;
	workspaceStatus: WorkspaceStatus;
}

/**
 * shouldConfirmAutostopRestart reports whether the user should be prompted to
 * restart their workspace so a newly saved autostop value applies immediately.
 *
 * The prompt is only meaningful when autostop is enabled after the change and
 * the workspace is running, because the deadline is derived at build start.
 * Enabling autostop (or changing its value while enabled) does not touch a
 * running build's deadline, so without a restart the new value only applies on
 * the next start. Disabling autostop clears the running build's deadline
 * server-side, so no restart is required in that case.
 */
export const shouldConfirmAutostopRestart = ({
	autostopChanged,
	autostopEnabled,
	workspaceStatus,
}: ShouldConfirmAutostopRestartOptions): boolean =>
	autostopChanged && autostopEnabled && workspaceStatus === "running";
