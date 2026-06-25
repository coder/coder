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
	onClick: () => void;
}

export const MCPServerRow: FC<MCPServerRowProps> = ({ server, onClick }) => {
	const clickableProps = useClickableTableRow({ onClick });
	const enabled = server.enabled;

	return (
		<TableRow {...clickableProps}>
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
				<Badge variant="default">{enabled ? "Enabled" : "Disabled"}</Badge>
			</TableCell>
			<TableCell className="w-12">
				<ChevronRightIcon className="size-5 text-content-primary" />
			</TableCell>
		</TableRow>
	);
};
