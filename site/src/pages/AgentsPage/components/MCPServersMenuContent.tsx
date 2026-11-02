import { cn } from "cn";
import { ArrowRightIcon, CheckIcon, LockIcon, ServerIcon } from "lucide-react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { DropdownMenuLabel } from "#/components/DropdownMenu/DropdownMenu";
import { menuItemClass } from "#/components/DropdownMenu/menuClasses";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	countMCPServersNeedingSignIn,
	getMCPMenuServers,
} from "./mcpMenuServers";

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

	return (
		<DropdownMenuPrimitive.CheckboxItem
			checked={isSelected}
			disabled={isDisabled || isForceOn || isDisconnected || !onToggle}
			onCheckedChange={(checked) => onToggle?.(server.id, checked === true)}
			onSelect={(event) => event.preventDefault()}
			className={cn(
				menuItemClass,
				"gap-2 py-1.5 text-xs leading-[18px] data-disabled:opacity-100",
			)}
		>
			<span className="flex size-5 shrink-0 items-center justify-center rounded-md border border-solid border-border/60 bg-surface-secondary [&>svg]:size-4 [&>img]:size-4">
				{server.icon_url ? (
					<ExternalImage src={server.icon_url} alt="" className="rounded-sm" />
				) : (
					<ServerIcon className="text-content-secondary" />
				)}
			</span>
			<span className="min-w-0 flex-1">
				<span
					className={cn(
						"flex items-center gap-1.5",
						isDisconnected ? "text-content-secondary" : "text-content-primary",
					)}
				>
					<span className="truncate">{server.display_name}</span>
					{isForceOn && (
						<LockIcon className="size-3 shrink-0 text-content-secondary" />
					)}
				</span>
				{isForceOn && !isDisconnected && (
					<span className="block text-xs font-normal text-content-secondary">
						Always on
					</span>
				)}
				{isForceOn && isDisconnected && (
					<span className="sr-only">Always on</span>
				)}
			</span>
			<span className="flex w-7 shrink-0 items-center justify-center">
				{isDisconnected ? (
					<Tooltip>
						<TooltipTrigger asChild>
							<StatusIndicatorDot
								role="img"
								aria-label="Not connected"
								variant="warning"
								className="pointer-events-auto shrink-0"
							/>
						</TooltipTrigger>
						<TooltipContent>Not connected</TooltipContent>
					</Tooltip>
				) : (
					<span
						aria-hidden="true"
						className={cn(
							"inline-flex h-4 w-7 shrink-0 items-center overflow-hidden rounded-full border border-solid transition-colors",
							(isDisabled || isForceOn || !onToggle) && "opacity-50",
							isSelected
								? "border-transparent bg-surface-invert-primary"
								: "border-border-control",
							(isDisabled || isForceOn || !onToggle) &&
								!isSelected &&
								"border-border",
						)}
					>
						<span
							className={cn(
								"block size-3 shrink-0 rounded-full transition-transform",
								isSelected
									? "translate-x-3.25 bg-surface-primary"
									: "translate-x-px bg-border-control",
								(isDisabled || isForceOn || !onToggle) &&
									!isSelected &&
									"bg-border",
							)}
						/>
					</span>
				)}
			</span>
		</DropdownMenuPrimitive.CheckboxItem>
	);
};

type MCPServersMenuContentProps = {
	servers: readonly TypesGen.MCPServerConfig[];
	selectedServerIds: readonly string[] | undefined;
	isDisabled: boolean;
	onToggle?: (serverId: string, checked: boolean) => void;
	onManageConnections?: () => void;
};

/**
 * Without `onToggle` the switches are read-only; without
 * `onManageConnections` the action is hidden.
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
		<>
			<DropdownMenuLabel className="px-2 pb-2 pt-1 text-content-primary">
				MCP servers
			</DropdownMenuLabel>
			{rows.length === 0 ? (
				<p className="m-0 px-2 py-1.5 text-xs text-content-secondary">
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
			<div className="-mx-2 -mb-2 mt-2 flex items-center justify-between gap-4 border-0 border-t border-solid border-border px-4 py-2 text-xs">
				<span className="flex items-center gap-1.5 whitespace-nowrap text-content-secondary">
					{disconnectedCount === 0 ? (
						<CheckIcon
							aria-hidden="true"
							className="size-3.5 text-content-success"
						/>
					) : (
						<span
							aria-hidden="true"
							className="size-1.5 rounded-full bg-content-warning"
						/>
					)}
					{disconnectedCount === 0
						? "All connected"
						: `${disconnectedCount} not connected`}
				</span>
				{onManageConnections && (
					<DropdownMenuPrimitive.Item asChild onSelect={onManageConnections}>
						<Button
							variant="outline"
							size="sm"
							className="ml-auto"
							aria-label="Manage MCP connections"
						>
							Manage
							<ArrowRightIcon />
						</Button>
					</DropdownMenuPrimitive.Item>
				)}
			</div>
		</>
	);
};
