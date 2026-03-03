import type { ComponentPropsWithRef } from "react";
import { cn } from "utils/cn";

type ThinkingProps = ComponentPropsWithRef<"div">;

export const Thinking = ({ className, ref, ...props }: ThinkingProps) => {
	return (
		<div
			ref={ref}
			className={cn(
				"rounded-lg border border-border bg-surface-primary px-3 py-2 text-xs text-content-secondary",
				className,
			)}
			{...props}
		/>
	);
};
