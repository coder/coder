import { type FC, useEffect, useEffectEvent, useRef, useState } from "react";
import { useQuery } from "react-query";
import { deploymentConfig } from "#/api/queries/deployment";
import { appearanceSettings } from "#/api/queries/users";
import { workspaceUsage } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { getTerminalConfig } from "#/modules/terminal/terminalConfig";
import type { ConnectionStatus } from "#/modules/terminal/types";
import {
	WorkspaceTerminal,
	type WorkspaceTerminalHandle,
} from "#/modules/terminal/WorkspaceTerminal";
import { WorkspaceTerminalAlerts } from "#/modules/terminal/WorkspaceTerminalAlerts";
import { openMaybePortForwardedURL } from "#/utils/portForward";

/**
 * Brief readiness delay for a freshly created terminal tab. The parent waits
 * for first output when it arrives quickly, but promotes the tab after this
 * ceiling so a silent connection still becomes reachable promptly.
 */
const READY_FALLBACK_MS = 100;

/** Keeps a recently hidden terminal attached long enough for quick tab toggles. */
const TERMINAL_IDLE_DETACH_MS = 30_000;

interface TerminalPanelProps {
	chatId: string;
	/** Used as the reconnection token so the PTY session survives navigation and page reloads. */
	reconnectionToken?: string;
	/** Whether this terminal should hold live xterm and WebSocket resources. */
	isHot?: boolean;
	/**
	 * Whether the terminal should grab keyboard focus. Gate this on the tab being
	 * the active tab, not merely connecting, so a terminal connecting off screen
	 * does not steal focus and focus moves to it once it is promoted to active.
	 */
	autoFocus?: boolean;
	/**
	 * Fires once the terminal is ready to be shown: the first output has
	 * painted, the connection dropped, or a brief fallback timeout elapsed. The
	 * parent uses this to defer activating a freshly created terminal tab long
	 * enough to avoid most empty-panel flashes without delaying silent terminals.
	 */
	onReady?: () => void;
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
}

export const TerminalPanel: FC<TerminalPanelProps> = ({
	chatId,
	reconnectionToken = chatId,
	isHot,
	autoFocus = true,
	onReady,
	workspace,
	workspaceAgent,
}) => {
	const { proxy } = useProxy();
	const { metadata } = useEmbeddedMetadata();
	const terminalRef = useRef<WorkspaceTerminalHandle>(null);
	const [isWarm, setIsWarm] = useState(Boolean(isHot));
	const [connectionStatus, setConnectionStatus] =
		useState<ConnectionStatus>("initializing");
	const detachTerminal = useEffectEvent(() => {
		setIsWarm(false);
		setConnectionStatus("initializing");
	});

	useEffect(() => {
		if (isHot) {
			setIsWarm(true);
			return;
		}
		if (!isWarm) {
			return;
		}

		const timer = setTimeout(detachTerminal, TERMINAL_IDLE_DETACH_MS);
		return () => clearTimeout(timer);
	}, [isHot, isWarm]);

	const shouldMountTerminal = Boolean(isHot) || isWarm;
	// A freshly created terminal tab connects off screen while the previous tab
	// stays visible. Notify the parent exactly once it is ready or the brief
	// fallback elapses, avoiding most empty-panel flashes without delaying silent
	// terminals. The first caller wins: painted output, a dropped connection, or
	// the fallback timeout.
	const hasSignaledReadyRef = useRef(false);
	const signalReady = useEffectEvent(() => {
		if (hasSignaledReadyRef.current) {
			return;
		}
		hasSignaledReadyRef.current = true;
		onReady?.();
	});
	const handleStatusChange = (status: ConnectionStatus) => {
		setConnectionStatus(status);
		// A dropped connection produces no output, so signal readiness to surface
		// the terminal alerts instead of waiting on the fallback timer.
		if (status === "disconnected") {
			signalReady();
		}
	};
	useEffect(() => {
		if (!shouldMountTerminal) {
			return;
		}

		const timer = setTimeout(signalReady, READY_FALLBACK_MS);
		return () => clearTimeout(timer);
	}, [shouldMountTerminal]);
	const config = useQuery(deploymentConfig());
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const terminalConfig = getTerminalConfig(
		config.data,
		appearanceSettingsQuery.data,
		proxy.preferredPathAppURL,
	);

	useQuery(
		workspaceUsage({
			usageApp: "reconnecting-pty",
			connectionStatus,
			workspaceId: shouldMountTerminal ? workspace?.id : undefined,
			agentId: shouldMountTerminal ? workspaceAgent?.id : undefined,
		}),
	);

	const handleOpenLink = (uri: string) => {
		openMaybePortForwardedURL(
			uri,
			proxy.preferredWildcardHostname,
			workspaceAgent?.name,
			workspace?.name,
			workspace?.owner_name,
		);
	};

	const handleTerminalError = (error: Error) => {
		console.error("WebSocket failed:", error);
	};

	const handleAlertChange = () => {
		terminalRef.current?.refit();
	};

	if (!workspaceAgent) {
		return (
			<div className="flex h-full min-h-0 flex-col">
				<div className="flex min-h-0 flex-1 items-center justify-center px-6 text-center text-xs text-content-secondary">
					Terminal will be available once the workspace agent is ready.
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<WorkspaceTerminalAlerts
				agent={workspaceAgent}
				status={connectionStatus}
				onAlertChange={handleAlertChange}
			/>
			<div className="min-h-0 flex-1">
				{shouldMountTerminal && (
					<WorkspaceTerminal
						ref={terminalRef}
						agentId={workspaceAgent.id}
						operatingSystem={workspaceAgent.operating_system}
						isVisible={shouldMountTerminal}
						autoFocus={Boolean(isHot) && autoFocus}
						onStatusChange={handleStatusChange}
						onContentReady={signalReady}
						onError={handleTerminalError}
						reconnectionToken={reconnectionToken}
						baseUrl={terminalConfig.baseUrl}
						terminalFontFamily={terminalConfig.fontFamily}
						renderer={terminalConfig.renderer}
						onOpenLink={handleOpenLink}
						loading={config.isLoading || appearanceSettingsQuery.isLoading}
						testId="agents-sidebar-terminal"
					/>
				)}
			</div>
		</div>
	);
};
