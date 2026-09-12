import { cn } from "cn";
import { ServerIcon } from "lucide-react";
import type { FC } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

const variantClasses = {
	tile: "rounded border border-solid border-border",
	circle: "rounded-full",
};

// Decorative: every consumer renders the server name as text or labels
// the containing control, so the icon itself carries no alt text.
export const MCPServerIcon: FC<{
	iconUrl: string;
	className?: string;
	variant?: keyof typeof variantClasses;
}> = ({ iconUrl, className, variant = "tile" }) => {
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center bg-surface-secondary",
				variantClasses[variant],
				className,
			)}
		>
			{iconUrl ? (
				<ExternalImage src={iconUrl} alt="" className="size-3/5" />
			) : (
				<ServerIcon className="size-3/5 text-content-secondary" />
			)}
		</div>
	);
};
