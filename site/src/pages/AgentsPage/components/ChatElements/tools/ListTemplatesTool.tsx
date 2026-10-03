import { ExternalLinkIcon } from "lucide-react";
import type React from "react";
import { Link } from "react-router";
import { ToolCall } from "./ToolCall";
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
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to list templates"}
			hasContent={hasContent}
		>
			<ToolCall.Header iconName="list_templates" label={label} />
			<ToolCall.Content>
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
								<div key={index} className="text-[13px] text-content-secondary">
									{templateName}
								</div>
							);
						}

						return (
							<div key={name} className="flex items-center gap-1.5">
								<Link
									to={`/templates/${name}`}
									className="flex items-center gap-1.5 text-[13px] text-content-secondary opacity-50 transition-opacity hover:opacity-100"
								>
									<span>{templateName}</span>
									<ExternalLinkIcon className="size-3 shrink-0" />
								</Link>
							</div>
						);
					})}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
