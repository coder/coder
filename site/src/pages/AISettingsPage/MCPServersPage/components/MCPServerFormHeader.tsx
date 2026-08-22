import {
	ArrowLeftIcon,
	EllipsisVerticalIcon,
	RefreshCwIcon,
	TrashIcon,
} from "lucide-react";
import { type FC, useId } from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { MCPServerIcon } from "./MCPServerIcon";

const MCPServerFormBackLink: FC<{ to: string }> = ({ to }) => {
	return (
		<Link to={to} className="-ml-3">
			<Button variant="subtle" type="button">
				<ArrowLeftIcon />
				<span>Back to MCP servers</span>
			</Button>
		</Link>
	);
};

interface MCPServerFormHeaderProps {
	server?: TypesGen.MCPServerConfig;
	title: string;
	iconUrl: string;
	listPath?: string;
	isEditing: boolean;
	isDisabled: boolean;
	onRequestDelete?: () => void;
	onRegenerateSigningSecret?: () => void;
	onToggleEnabled?: (enabled: boolean) => void;
}

export const MCPServerFormHeader: FC<MCPServerFormHeaderProps> = ({
	server,
	title,
	iconUrl,
	listPath,
	isEditing,
	isDisabled,
	onRequestDelete,
	onRegenerateSigningSecret,
	onToggleEnabled,
}) => {
	const disabledReasonId = useId();
	const lacksUpdatePermission = !onToggleEnabled;

	return (
		<>
			<div className="flex items-center justify-between">
				{listPath && <MCPServerFormBackLink to={listPath} />}
				{isEditing &&
					server &&
					(onRequestDelete ||
						(server.has_signing_secret && onRegenerateSigningSecret)) && (
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									variant="subtle"
									size="icon"
									type="button"
									disabled={isDisabled}
									aria-label="Server actions"
								>
									<EllipsisVerticalIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								{server.has_signing_secret && onRegenerateSigningSecret && (
									<DropdownMenuItem onClick={onRegenerateSigningSecret}>
										<RefreshCwIcon />
										Regenerate signing secret
									</DropdownMenuItem>
								)}
								{onRequestDelete && (
									<DropdownMenuItem
										className="text-content-destructive focus:text-content-destructive"
										onClick={onRequestDelete}
									>
										<TrashIcon />
										Remove
									</DropdownMenuItem>
								)}
							</DropdownMenuContent>
						</DropdownMenu>
					)}
			</div>
			<div className="flex items-center justify-between gap-4">
				<div className="flex min-w-0 items-center gap-4">
					{isEditing && (
						<MCPServerIcon iconUrl={iconUrl} name={title} className="size-12" />
					)}
					<SettingsHeaderTitle>
						<span
							className={cn(
								"block min-w-0 truncate",
								server?.enabled === false && "text-content-secondary",
							)}
						>
							{title}
						</span>
					</SettingsHeaderTitle>
					{isEditing && server && !server.enabled && (
						<Badge variant="default">Disabled</Badge>
					)}
					{isEditing &&
						server?.forward_coder_headers &&
						server.has_signing_secret && (
							<Badge variant="default">Signing enabled</Badge>
						)}
				</div>
				{isEditing && server && (
					<div className="flex shrink-0 items-center gap-2">
						<Tooltip>
							<TooltipTrigger asChild>
								<span className="inline-flex">
									<Switch
										checked={server.enabled}
										onCheckedChange={(checked) => {
											if (onToggleEnabled) {
												onToggleEnabled(checked);
											}
										}}
										disabled={isDisabled}
										aria-disabled={lacksUpdatePermission}
										aria-describedby={
											lacksUpdatePermission ? disabledReasonId : undefined
										}
										aria-label="Server enabled"
										className="aria-disabled:cursor-not-allowed aria-disabled:data-[state=checked]:bg-surface-tertiary aria-disabled:data-[state=unchecked]:bg-surface-tertiary"
									/>
								</span>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								{lacksUpdatePermission
									? "You do not have permission to update this server."
									: server.enabled
										? "Disable this server. It will be hidden from agents."
										: "Enable this server. It will be visible to agents."}
							</TooltipContent>
						</Tooltip>
						{lacksUpdatePermission && (
							<span id={disabledReasonId} className="sr-only">
								You do not have permission to update this server.
							</span>
						)}
						<span className="text-sm">Enable</span>
					</div>
				)}
			</div>
		</>
	);
};
