import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";
import { ToolCollapsible } from "./ToolCollapsible";
import { asRecord, asString, type ToolStatus } from "./utils";

/**
 * Collapsed-by-default rendering for `list_templates` tool calls. Shows
 * "Listed N templates" with a chevron; expanding reveals the template list.
 */
export const ListTemplatesTool: React.FC<{
	templates: unknown[];
	count: number;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ templates, count, status, isError, errorMessage }) => {
	const hasContent = templates.length > 0;
	const isRunning = status === "running";

	const label =
		isRunning || count === 0
			? "Listing templates…"
			: count === 1
				? "Listed 1 template"
				: `Listed ${count} templates`;

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasContent}
			header={
				<>
					<span
						className={cn(
							"text-sm",
							isError
								? "text-content-destructive"
								: "text-content-secondary",
						)}
					>
						{label}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to list templates"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			<ScrollArea
				className="mt-1.5 rounded-md border border-solid border-border-default"
				viewportClassName="max-h-64"
				scrollBarClassName="w-1.5"
			>
				<div className="px-3 py-2">
					<ul className="m-0 list-none space-y-2">
						{templates.map((template, index) => {
							const rec = asRecord(template);
							if (!rec) {
								return null;
							}
							const id = asString(rec.id);
							const name = asString(rec.name);
							const displayName = asString(rec.display_name);
							const description = asString(rec.description);
							const templateName = displayName || name || `Template ${index + 1}`;

							return (
								<li key={id || index} className="border-b border-solid border-border-default pb-2 last:border-b-0 last:pb-0">
									<div className="font-medium text-content-primary">
										{templateName}
									</div>
									{description && (
										<div className="mt-0.5 text-xs text-content-secondary">
											{description}
										</div>
									)}
									{id && (
										<div className="mt-0.5 font-mono text-xs text-content-secondary opacity-60">
											{id}
										</div>
									)}
								</li>
							);
						})}
					</ul>
				</div>
			</ScrollArea>
		</ToolCollapsible>
	);
};
