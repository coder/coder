import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { ServerIcon } from "lucide-react";
import type { FC } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

const iconVariants = cva(
	"flex shrink-0 items-center justify-center bg-surface-secondary",
	{
		variants: {
			variant: {
				tile: "rounded border border-solid border-border",
				circle: "rounded-full",
			},
		},
		defaultVariants: {
			variant: "tile",
		},
	},
);

export const MCPServerIcon: FC<
	VariantProps<typeof iconVariants> & {
		iconUrl: string;
		className?: string;
	}
> = ({ iconUrl, className, variant }) => {
	return (
		<div className={cn(iconVariants({ variant }), className)}>
			{iconUrl ? (
				<ExternalImage src={iconUrl} alt="" className="size-3/5" />
			) : (
				<ServerIcon className="size-3/5 text-content-secondary" />
			)}
		</div>
	);
};
