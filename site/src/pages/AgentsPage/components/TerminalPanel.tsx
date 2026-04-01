import { type FC, useRef, useState } from "react";
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

interface TerminalPanelProps {
	/** Used as the reconnection token so the PTY session survives
	 * navigation and page reloads. */
	chatId: string;
	isVisible?: boolean;
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
}

export const TerminalPanel: FC<TerminalPanelProps> = ({
	chatId,
	isVisible,
	workspace,
	workspaceAgent,
}) => {
	const { proxy } = useProxy();
	const { metadata } = useEmbeddedMetadata();
	const terminalRef = useRef<WorkspaceTerminalHandle>(null);
	const [connectionStatus, setConnectionStatus] =
		useState<ConnectionStatus>("initializing");
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
			workspaceId: workspace?.id,
			agentId: workspaceAgent?.id,
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
				<WorkspaceTerminal
					ref={terminalRef}
					agentId={workspaceAgent.id}
					operatingSystem={workspaceAgent.operating_system}
					autoFocus={false}
					isVisible={isVisible}
					onStatusChange={setConnectionStatus}
					onError={handleTerminalError}
					reconnectionToken={chatId}
					baseUrl={terminalConfig.baseUrl}
					terminalFontFamily={terminalConfig.fontFamily}
					renderer={terminalConfig.renderer}
					onOpenLink={handleOpenLink}
					loading={config.isLoading || appearanceSettingsQuery.isLoading}
					testId="agents-sidebar-terminal"
				/>
			</div>
		</div>
	);
};
