import { ChevronRightIcon } from "lucide-react";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { MCPServerIcon } from "#/modules/mcpServers/MCPServerIcon";
import {
	AUTH_TYPE_LABELS,
	AVAILABILITY_LABELS,
	DISABLED_AVAILABILITY_LABEL,
} from "./mcpServerFormLogic";

type MCPServerRowProps = {
	server: TypesGen.MCPServerConfig;
	onClick?: () => void;
};

export const MCPServerRow: React.FC<MCPServerRowProps> = ({
	server,
	onClick,
}) => {
	const clickableProps = useClickableTableRow({
		onClick: () => onClick?.(),
	});

	return (
		<TableRow {...(onClick ? clickableProps : {})}>
			<TableCell className="min-w-0 px-4 py-3">
				<div className="flex min-w-0 items-center gap-3">
					<MCPServerIcon iconUrl={server.icon_url} className="size-10" />
					<span className="truncate text-sm font-medium text-content-primary">
						{server.display_name}
					</span>
					{!server.enabled && (
						<Badge asChild size="sm" variant="default" className="shrink-0">
							<span>Disabled</span>
						</Badge>
					)}
				</div>
			</TableCell>
			<TableCell className="text-sm">
				{AUTH_TYPE_LABELS[server.auth_type] ?? server.auth_type}
			</TableCell>
			<TableCell className="text-sm">
				{server.enabled
					? (AVAILABILITY_LABELS[server.availability] ?? server.availability)
					: DISABLED_AVAILABILITY_LABEL}
			</TableCell>
			<TableCell className="w-10 text-center">
				{onClick && (
					<div className="flex items-center justify-end pr-4">
						<ChevronRightIcon
							aria-hidden
							className="size-icon-sm shrink-0 text-content-secondary"
						/>
					</div>
				)}
			</TableCell>
		</TableRow>
	);
};
