import { cn } from "cn";
import { ServerIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

// Decorative: every consumer renders the server name as text or labels
// the containing control, so the icon itself carries no alt text.
export const MCPServerIcon: FC<{
	iconUrl: string;
	className?: string;
}> = ({ iconUrl, className }) => {
	const icon = iconUrl ? (
		<ExternalImage src={iconUrl} alt="" className="size-3/5" />
	) : (
		<ServerIcon className="size-3/5 text-content-secondary" />
	);

	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
				className,
			)}
		>
			{icon}
		</div>
	);
};

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
					<MCPServerIcon iconUrl={s.icon_url} className="size-4" />
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
