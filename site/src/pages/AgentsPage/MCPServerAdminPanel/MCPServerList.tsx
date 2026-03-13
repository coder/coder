import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	CheckCircleIcon,
	ChevronRightIcon,
	CircleIcon,
	PlusIcon,
	ServerIcon,
} from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

interface MCPServerListProps {
	servers: TypesGen.ChatMCPServerConfig[];
	isLoading: boolean;
	onAdd: () => void;
	onEdit: (server: TypesGen.ChatMCPServerConfig) => void;
}

export const MCPServerList: FC<MCPServerListProps> = ({
	servers,
	isLoading,
	onAdd,
	onEdit,
}) => {
	if (isLoading) {
		return (
			<div className="rounded-lg border border-dashed border-border bg-surface-primary p-6 text-center text-[13px] text-content-secondary">
				Loading MCP servers...
			</div>
		);
	}

	return (
		<div className="space-y-3">
			<div className="flex items-center justify-end">
				<Button size="sm" variant="outline" onClick={onAdd}>
					<PlusIcon className="h-4 w-4" />
					Add Server
				</Button>
			</div>

			{servers.length === 0 ? (
				<div className="rounded-lg border border-dashed border-border bg-surface-primary p-6 text-center text-[13px] text-content-secondary">
					No MCP servers configured yet. Add a server to extend agent
					capabilities.
				</div>
			) : (
				<div className="overflow-hidden rounded-lg border border-border">
					{servers.map((server, i) => (
						<button
							type="button"
							key={server.id}
							aria-label={server.display_name || server.slug}
							onClick={() => onEdit(server)}
							className={cn(
								"flex w-full cursor-pointer items-center gap-3.5 border-0 bg-transparent px-3 py-3 text-left transition-colors hover:bg-surface-secondary/30",
								i > 0 && "border-0 border-t border-solid border-border/50",
							)}
						>
							<ServerIcon className="h-5 w-5 shrink-0 text-content-secondary" />
							<div className="min-w-0 flex-1">
								<span className="block truncate text-[14px] font-medium text-content-primary">
									{server.display_name || server.slug}
								</span>
								<span className="block truncate text-[12px] text-content-secondary">
									{server.url}
								</span>
							</div>
							{server.enabled ? (
								<CheckCircleIcon className="h-4 w-4 shrink-0 text-content-success" />
							) : (
								<CircleIcon className="h-4 w-4 shrink-0 text-content-secondary opacity-40" />
							)}
							<ChevronRightIcon className="h-5 w-5 shrink-0 text-content-secondary" />
						</button>
					))}
				</div>
			)}
		</div>
	);
};
