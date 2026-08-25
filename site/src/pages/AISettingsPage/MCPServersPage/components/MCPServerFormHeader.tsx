import { ArrowLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { Switch } from "#/components/Switch/Switch";
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
	onToggleEnabled,
}) => {
	return (
		<>
			<div className="flex items-center justify-between">
				{listPath && <MCPServerFormBackLink to={listPath} />}
				{isEditing && server && onRequestDelete && (
					<Button
						type="button"
						variant="destructive"
						disabled={isDisabled}
						onClick={onRequestDelete}
					>
						<span>Delete</span>
					</Button>
				)}
			</div>
			<div className="flex items-center gap-4 pt-6 min-w-0">
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
				<div className="flex items-center justify-between w-full pt-6">
					<p className="text-sm text-content-secondary m-0">
						Disabled servers are hidden from agents.
					</p>
					<div className="flex shrink-0 items-center gap-2">
						<Switch
							checked={server.enabled}
							onCheckedChange={(checked) => {
								onToggleEnabled?.(checked);
							}}
							disabled={isDisabled || !onToggleEnabled}
							aria-label="Server enabled"
						/>
						<span className="text-sm">Enable</span>
					</div>
				</div>
			)}
		</>
	);
};
