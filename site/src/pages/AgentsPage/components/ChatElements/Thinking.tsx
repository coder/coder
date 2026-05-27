import { BrainIcon } from "lucide-react";
import type { ComponentPropsWithRef } from "react";
import { cn } from "#/utils/cn";

type ThinkingProps = ComponentPropsWithRef<"div">;

export const Thinking = ({ className, ref, ...props }: ThinkingProps) => {
	return (
		<div
			ref={ref}
			className={cn(
				"flex items-start gap-2 rounded-lg border border-border bg-surface-primary px-3 py-2 text-xs text-content-secondary",
				className,
			)}
			{...props}
		>
			<BrainIcon className="mt-0.5 size-4 shrink-0 stroke-[1.5] text-current" />
			<div className="min-w-0 flex-1">{props.children}</div>
		</div>
	);
};
