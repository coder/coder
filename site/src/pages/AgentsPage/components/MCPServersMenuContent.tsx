import { ArrowRightIcon, LockIcon, ServerIcon } from "lucide-react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Separator } from "#/components/Separator/Separator";
import {
	StatusIndicator,
	StatusIndicatorDot,
} from "#/components/StatusIndicator/StatusIndicator";
import { Switch } from "#/components/Switch/Switch";
import {
	countMCPServersNeedingSignIn,
	getMCPMenuServers,
} from "../utils/mcpSelection";

type MCPServerMenuRowProps = {
	server: TypesGen.MCPServerConfig;
	isSelected: boolean;
	isDisconnected: boolean;
	isDisabled: boolean;
	onToggle?: (serverId: string, checked: boolean) => void;
};

const MCPServerMenuRow: React.FC<MCPServerMenuRowProps> = ({
	server,
	isSelected,
	isDisconnected,
	isDisabled,
	onToggle,
}) => {
	const isForceOn = server.availability === "force_on";
	let switchLabel = `Enable ${server.display_name}`;
	if (isForceOn) {
		switchLabel = `${server.display_name} always on`;
	} else if (isSelected) {
		switchLabel = `Disable ${server.display_name}`;
	}

	return (
		<div className="flex min-h-8 items-center gap-1.5 px-1 py-1.5">
			{server.icon_url ? (
				<ExternalImage
					src={server.icon_url}
					alt=""
					className="size-3.5 shrink-0 rounded-sm"
				/>
			) : (
				<ServerIcon className="size-3.5 shrink-0 text-content-secondary" />
			)}
			<span className="min-w-0 flex-1 truncate text-xs text-content-secondary">
				{server.display_name}
			</span>
			{isForceOn && (
				<LockIcon className="size-3 shrink-0 text-content-secondary" />
			)}
			{isDisconnected ? (
				<StatusIndicator
					variant="warning"
					size="sm"
					className="shrink-0 font-normal"
				>
					<StatusIndicatorDot />
					Disconnected
				</StatusIndicator>
			) : (
				<Switch
					size="sm"
					checked={isSelected}
					onCheckedChange={(checked) => onToggle?.(server.id, checked)}
					disabled={isDisabled || isForceOn || !onToggle}
					aria-label={switchLabel}
				/>
			)}
		</div>
	);
};

type MCPServersMenuContentProps = {
	/** Enabled servers of the chat's organization, in display order. */
	servers: readonly TypesGen.MCPServerConfig[];
	selectedServerIds: readonly string[] | undefined;
	isDisabled: boolean;
	onToggle?: (serverId: string, checked: boolean) => void;
	onManageConnections?: () => void;
};

/**
 * Lists the chat's MCP servers with one switch each and links to the MCP
 * connections settings page, where users sign in and out. Without
 * `onToggle` the switches are read-only; without `onManageConnections`
 * the link is hidden.
 */
export const MCPServersMenuContent: React.FC<MCPServersMenuContentProps> = ({
	servers,
	selectedServerIds,
	isDisabled,
	onToggle,
	onManageConnections,
}) => {
	const rows = getMCPMenuServers(servers, selectedServerIds);
	const disconnectedCount = countMCPServersNeedingSignIn(servers);

	return (
		<div role="group" aria-label="MCP servers" className="flex flex-col">
			{rows.length === 0 ? (
				<p className="m-0 px-1 py-1.5 text-xs text-content-secondary">
					No servers connected yet
				</p>
			) : (
				rows.map((row) => (
					<MCPServerMenuRow
						key={row.server.id}
						{...row}
						isDisabled={isDisabled}
						onToggle={onToggle}
					/>
				))
			)}
			<Separator className="my-1" />
			<div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-1 py-0.5">
				<span className="whitespace-nowrap text-xs text-content-secondary">
					{disconnectedCount === 0
						? "All connected"
						: `${disconnectedCount} not connected`}
				</span>
				{onManageConnections && (
					<Button
						variant="outline"
						size="sm"
						className="ml-auto"
						onClick={onManageConnections}
					>
						Manage connections
						<ArrowRightIcon />
					</Button>
				)}
			</div>
		</div>
	);
};
