import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, ExternalLinkIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { Link } from "react-router";
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
							isError ? "text-content-destructive" : "text-content-secondary",
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
			<div className="mt-1.5">
				{templates.map((template, index) => {
					const rec = asRecord(template);
					if (!rec) {
						return null;
					}
					const name = asString(rec.name);
					const displayName = asString(rec.display_name);
					const templateName = displayName || name || `Template ${index + 1}`;

					if (!name) {
						return (
							<div key={index} className="text-sm text-content-secondary">
								{templateName}
							</div>
						);
					}

					return (
						<div key={name} className="flex items-center gap-1.5">
							<Link
								to={`/templates/${name}`}
								onClick={(e) => e.stopPropagation()}
								className="flex items-center gap-1.5 text-sm text-content-secondary opacity-50 transition-opacity hover:opacity-100"
							>
								<span>{templateName}</span>
								<ExternalLinkIcon className="h-3 w-3 shrink-0" />
							</Link>
						</div>
					);
				})}
			</div>
		</ToolCollapsible>
	);
};
