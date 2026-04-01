/**
 * Copied from shadc/ui on 06/20/2025
 * @see {@link https://ui.shadcn.com/docs/components/skeleton}
 */
import { cn } from "utils/cn";

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="skeleton"
			className={cn("bg-surface-tertiary animate-pulse rounded-md", className)}
			{...props}
		/>
	);
}

export { Skeleton };
