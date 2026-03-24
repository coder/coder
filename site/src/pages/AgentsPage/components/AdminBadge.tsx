import { ShieldIcon } from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

interface AdminBadgeProps {
	className?: string;
}

export const AdminBadge: FC<AdminBadgeProps> = ({ className }) => (
	<TooltipProvider delayDuration={0}>
		<Tooltip>
			<TooltipTrigger asChild>
				<span
					className={cn(
						"inline-flex cursor-default items-center gap-1 rounded bg-surface-tertiary/60 px-2 py-1 text-[11px] leading-none font-medium text-content-secondary",
						className,
					)}
				>
					<ShieldIcon className="h-3 w-3" />
					Admin
				</span>
			</TooltipTrigger>
			<TooltipContent side="right">
				Only visible to deployment administrators.
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);
