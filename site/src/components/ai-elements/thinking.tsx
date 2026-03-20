import type { ComponentPropsWithRef } from "react";
import { cn } from "utils/cn";

type ThinkingProps = ComponentPropsWithRef<"div">;

export const Thinking = ({ className, ref, ...props }: ThinkingProps) => {
	return (
		<div
			ref={ref}
			className={cn(
				"rounded-md border-l-2 border-content-secondary/40 bg-surface-secondary/30 px-3 py-1.5 text-[13px] text-content-secondary",
				className,
			)}
			{...props}
		/>
	);
};
