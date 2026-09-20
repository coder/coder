import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { MCPServerIcon } from "#/modules/mcpServers/MCPServerIcon";

const ICON_STACK_MAX = 3;

export const MCPServerIconStack: FC<{
	servers: readonly TypesGen.MCPServerConfig[];
}> = ({ servers }) => {
	const visible = servers.slice(0, ICON_STACK_MAX);
	return (
		<span className="inline-flex items-center">
			{visible.map((server) => (
				<span
					key={server.id}
					className="inline-flex rounded-full ring-1 ring-surface-primary not-first:-ml-1.5"
				>
					<MCPServerIcon
						iconUrl={server.icon_url}
						variant="circle"
						className="size-4"
					/>
				</span>
			))}
			{servers.length > ICON_STACK_MAX && (
				<span className="-ml-1 inline-flex size-4 items-center justify-center rounded-full bg-surface-secondary text-[9px] font-medium text-content-secondary ring-1 ring-surface-primary">
					+{servers.length - ICON_STACK_MAX}
				</span>
			)}
		</span>
	);
};
