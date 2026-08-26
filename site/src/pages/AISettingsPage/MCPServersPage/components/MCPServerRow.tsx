import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { cn } from "#/utils/cn";
import { MCPServerIcon } from "./MCPServerIcon";
import { AUTH_TYPE_LABELS, AVAILABILITY_LABELS } from "./mcpServerFormLogic";

interface MCPServerRowProps {
	server: TypesGen.MCPServerConfig;
	onClick?: () => void;
}

export const MCPServerRow: FC<MCPServerRowProps> = ({ server, onClick }) => {
	const clickableProps = useClickableTableRow({
		onClick: () => onClick?.(),
	});
	const enabled = server.enabled;

	return (
		<TableRow {...(onClick ? clickableProps : {})}>
			<TableCell className="min-w-0 px-4 py-3">
				<div className="flex min-w-0 items-center gap-3">
					<MCPServerIcon
						iconUrl={server.icon_url}
						name={server.display_name}
						className="size-10"
					/>
					<span
						className={cn(
							"truncate text-sm font-medium",
							enabled ? "text-content-primary" : "text-content-secondary",
						)}
					>
						{server.display_name}
					</span>
				</div>
			</TableCell>
			<TableCell className="w-1/5 text-sm">
				{AUTH_TYPE_LABELS[server.auth_type] ?? server.auth_type}
			</TableCell>
			<TableCell className="w-1/5 text-sm">
				{AVAILABILITY_LABELS[server.availability] ?? server.availability}
			</TableCell>
			<TableCell className="w-32">
				{enabled ? (
					<Badge variant="green">Enabled</Badge>
				) : (
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Badge variant="warning">Disabled</Badge>
							</TooltipTrigger>
							<TooltipContent>
								This server is disabled and won't be available to Coder Agents.
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				)}
			</TableCell>
			<TableCell className="w-12">
				{onClick && (
					<ChevronRightIcon className="size-5 text-content-primary" />
				)}
			</TableCell>
		</TableRow>
	);
};
