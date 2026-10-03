import { cn } from "cn";
import { InfoIcon } from "lucide-react";

// Avoid announcing historical hook notices as live alerts.
export const TimelineNotice: React.FC<React.ComponentProps<"div">> = ({
	children,
	className,
	...props
}) => (
	<div
		role="note"
		{...props}
		className={cn(
			"relative my-1 w-full rounded-lg border border-solid border-border-default bg-surface-secondary p-4 text-left",
			className,
		)}
	>
		<div className="flex min-w-0 flex-1 flex-row items-start gap-3 text-sm">
			<InfoIcon className="size-icon-sm mt-[3px] text-highlight-sky" />
			<div className="min-w-0 flex-1">{children}</div>
		</div>
	</div>
);
