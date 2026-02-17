import { forwardRef } from "react";
import { cn } from "utils/cn";

type ThinkingProps = React.HTMLAttributes<HTMLDivElement>;

export const Thinking = forwardRef<HTMLDivElement, ThinkingProps>(
	({ className, ...props }, ref) => {
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
	},
);

Thinking.displayName = "Thinking";
