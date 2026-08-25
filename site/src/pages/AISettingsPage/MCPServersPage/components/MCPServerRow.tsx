import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
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
						className={cn("size-10", !enabled && "opacity-50")}
					/>
					<span
						className={cn(
							"truncate text-sm font-medium",
							enabled ? "text-content-primary" : "text-content-disabled",
						)}
					>
						{server.display_name}
					</span>
					{!enabled && (
						<Badge variant="default" className="shrink-0">
							Disabled
						</Badge>
					)}
				</div>
			</TableCell>
			<TableCell
				className={cn("w-1/5 text-sm", !enabled && "text-content-disabled")}
			>
				{AUTH_TYPE_LABELS[server.auth_type] ?? server.auth_type}
			</TableCell>
			<TableCell
				className={cn("w-1/5 text-sm", !enabled && "text-content-disabled")}
			>
				{AVAILABILITY_LABELS[server.availability] ?? server.availability}
			</TableCell>
			<TableCell className="w-12">
				{onClick && (
					<ChevronRightIcon className="size-5 text-content-primary" />
				)}
			</TableCell>
		</TableRow>
	);
};
