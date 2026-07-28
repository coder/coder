import { ArrowLeftIcon, EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import type { FC } from "react";
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

const MCPServerFormBackLink: FC = () => {
	return (
		<Link to="/ai/settings/mcp-servers" className="-ml-3">
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
	isEditing: boolean;
	isDisabled: boolean;
	onRequestDelete: () => void;
	onToggleEnabled?: (enabled: boolean) => void;
}

export const MCPServerFormHeader: FC<MCPServerFormHeaderProps> = ({
	server,
	title,
	iconUrl,
	isEditing,
	isDisabled,
	onRequestDelete,
	onToggleEnabled,
}) => {
	return (
		<>
			<div className="flex items-center justify-between">
				<MCPServerFormBackLink />
				{isEditing && server && (
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
							<DropdownMenuItem
								className="text-content-destructive focus:text-content-destructive"
								onClick={onRequestDelete}
							>
								<TrashIcon />
								Remove
							</DropdownMenuItem>
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
				</div>
				{isEditing && server && (
					<div className="flex shrink-0 items-center gap-2">
						<Tooltip>
							<TooltipTrigger asChild>
								<span className="inline-flex">
									<Switch
										checked={server.enabled}
										onCheckedChange={(checked) => onToggleEnabled?.(checked)}
										disabled={isDisabled}
										aria-label="Server enabled"
									/>
								</span>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								{server.enabled
									? "Disable this server. It will be hidden from agents."
									: "Enable this server. It will be visible to agents."}
							</TooltipContent>
						</Tooltip>
						<span className="text-sm">Enable</span>
					</div>
				)}
			</div>
		</>
	);
};
