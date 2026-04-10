import { ShieldIcon } from "lucide-react";
import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export const AdminBadge: FC = () => (
	<TooltipProvider delayDuration={0}>
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge variant="default" size="sm" className="ml-auto cursor-default">
					<ShieldIcon className="h-3 w-3" />
					Admin
				</Badge>
			</TooltipTrigger>
			<TooltipContent side="right">
				Only visible to deployment administrators.
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);
