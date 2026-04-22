import {
	LayoutDashboardIcon,
	LoaderIcon,
	TriangleAlertIcon,
} from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { ToolCollapsible } from "./ToolCollapsible";
import type { ToolStatus } from "./utils";

interface RenderForUserToolProps {
	title: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}

export const RenderForUserTool: FC<RenderForUserToolProps> = ({
	title,
	status,
	isError,
	errorMessage,
}) => {
	const isRunning = status === "running";

	const label = isRunning
		? "Rendering view…"
		: isError
			? "Failed to render view"
			: `Rendered view: ${title || "Untitled"}`;

	return (
		<ToolCollapsible
			hasContent={false}
			header={
				<>
					<LayoutDashboardIcon className="h-4 w-4 shrink-0 text-content-secondary" />
					<span className="truncate text-sm text-content-secondary">
						{label}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to render view"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			{null}
		</ToolCollapsible>
	);
};
