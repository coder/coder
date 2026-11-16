import type * as TypesGen from "#/api/typesGenerated";
import { MCPServerIcon } from "#/modules/mcpServers/MCPServerIcon";

const ICON_STACK_MAX = 3;

type MCPServerIconStackProps = {
	servers: readonly TypesGen.MCPServerConfig[];
};

export const MCPServerIconStack: React.FC<MCPServerIconStackProps> = ({
	servers,
}) => {
	return (
		<span className="inline-flex items-center">
			{servers.slice(0, ICON_STACK_MAX).map((server) => (
				<span
					key={server.id}
					className="relative z-0 inline-flex rounded-full before:absolute before:-inset-0.5 not-first:-ml-1.5 bg-surface-secondary border border-surface-primary"
				>
					<MCPServerIcon
						iconUrl={server.icon_url}
						variant="circle"
						className="size-4"
					/>
				</span>
			))}
			{servers.length > ICON_STACK_MAX && (
				<span className="relative z-0 -ml-1.5 inline-flex size-icon-sm items-center justify-center rounded-full text-2xs font-medium text-content-secondary border border-surface-primary">
					<span className="inline-flex size-full items-center justify-center rounded-full bg-surface-secondary">
						+{servers.length - ICON_STACK_MAX}
					</span>
				</span>
			)}
		</span>
	);
};
