import type { ChatAutomationNameMap } from "#/api/queries/chatAutomations";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export type ChatAutomationNames = {
	names: ChatAutomationNameMap;
	// State of the automations list: while "loading" (including refetches)
	// a missing name may still resolve, and "error" means the list failed.
	status: "loading" | "error" | "settled";
};

type AutomationLabelProps = {
	automationId: string;
	inputId?: string;
	automationName?: string;
	nameStatus?: ChatAutomationNames["status"];
};

export const AutomationLabel: React.FC<AutomationLabelProps> = ({
	automationId,
	inputId,
	automationName,
	nameStatus = "settled",
}) => {
	const inputPart = inputId ? ` · input ${inputId.slice(0, 8)}` : "";
	// Hide the ID while the name may still resolve so the label does not
	// flash the UUID.
	const showNameLoading = !automationName && nameStatus === "loading";
	const nameOrId = showNameLoading ? "" : (automationName ?? automationId);
	let statusText = "Automation name unavailable.";
	if (automationName) {
		statusText = `Automation: ${automationName}`;
	} else if (showNameLoading) {
		statusText = "Loading automation name.";
	} else if (nameStatus === "error") {
		statusText = "Could not load the automation name.";
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge
					asChild
					size="sm"
					variant="outline"
					className="min-w-0 max-w-full gap-0"
				>
					{/* The aria-label keeps the separator spaces that the split
					    spans would drop from the accessible name. */}
					<button
						type="button"
						className="cursor-default"
						aria-label={`Automation run${nameOrId ? ` · ${nameOrId}` : ""}${inputPart}`}
					>
						<span className="shrink-0 whitespace-pre">Automation run</span>
						{nameOrId && (
							<>
								<span className="shrink-0 whitespace-pre"> · </span>
								<span className="min-w-0 truncate">{nameOrId}</span>
							</>
						)}
						{inputPart && (
							<span className="shrink-0 whitespace-pre">{inputPart}</span>
						)}
					</button>
				</Badge>
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs break-words">
				<div>{statusText}</div>
				<div className="font-mono">Automation ID: {automationId}</div>
				{inputId && <div className="font-mono">Input ID: {inputId}</div>}
			</TooltipContent>
		</Tooltip>
	);
};
