import type * as TypesGen from "api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	Building2Icon,
	GlobeIcon,
	SquareArrowOutUpRightIcon,
	UsersIcon,
} from "lucide-react";

interface ShareIconProps {
	app: TypesGen.WorkspaceApp;
}

export const ShareIcon = ({ app }: ShareIconProps) => {
	if (app.external) {
		return (
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<SquareArrowOutUpRightIcon />
					</TooltipTrigger>
					<TooltipContent>Open external URL</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}
	if (app.sharing_level === "authenticated") {
		return (
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<UsersIcon />
					</TooltipTrigger>
					<TooltipContent>Shared with all authenticated users</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}
	if (app.sharing_level === "organization") {
		return (
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<Building2Icon />
					</TooltipTrigger>
					<TooltipContent>Shared with organization members</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}
	if (app.sharing_level === "public") {
		return (
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<GlobeIcon />
					</TooltipTrigger>
					<TooltipContent>Shared publicly</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return null;
};
