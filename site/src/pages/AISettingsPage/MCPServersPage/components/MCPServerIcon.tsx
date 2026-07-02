import { ServerIcon } from "lucide-react";
import type { FC } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { cn } from "#/utils/cn";

export const MCPServerIcon: FC<{
	iconUrl: string;
	name: string;
	className?: string;
}> = ({ iconUrl, name, className }) => {
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded bg-surface-secondary border border-solid border-border",
				className,
			)}
		>
			{iconUrl ? (
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className="size-3/5"
				/>
			) : (
				<ServerIcon className="size-3/5 text-content-secondary" />
			)}
		</div>
	);
};
