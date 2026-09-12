import { cn } from "cn";
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
			{visible.map((s, i) => (
				<span
					key={s.id}
					className={cn(
						"inline-flex rounded-full ring-1 ring-surface-primary",
						i > 0 && "-ml-1.5",
					)}
				>
					<MCPServerIcon
						iconUrl={s.icon_url}
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
