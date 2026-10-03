import { cn } from "cn";
import type { ChatAutomationReferenceMap } from "#/api/queries/chatAutomations";
import type { ChatAutomationReference } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { TimelineNotice } from "./TimelineNotice";

export type ChatAutomationReferences = {
	references: ChatAutomationReferenceMap;
	// State of the chat's automation references: while "loading" (including
	// refetches) a missing name may still resolve, and "error" means the
	// request failed.
	status: "loading" | "error" | "settled";
};

const labelSeparator = " · ";

type AutomationLabelProps = {
	automationId: string;
	inputId?: string;
	reference?: ChatAutomationReference;
	nameStatus: ChatAutomationReferences["status"];
	/** "card": full-width info card; "badge": compact inline badge. */
	variant: "card" | "badge";
};

export const AutomationLabel: React.FC<AutomationLabelProps> = ({
	automationId,
	inputId,
	reference,
	nameStatus,
	variant,
}) => {
	const showNameLoading = !reference && nameStatus === "loading";
	const nameOrId = showNameLoading ? "" : (reference?.name ?? automationId);
	const kindSuffix = reference ? ` (${reference.kind})` : "";
	const suffix = nameOrId ? `${labelSeparator}${nameOrId}${kindSuffix}` : "";
	let statusText = "Automation name unavailable.";
	if (reference) {
		statusText = `Automation: ${nameOrId}${kindSuffix}`;
	} else if (showNameLoading) {
		statusText = "Loading automation name.";
	} else if (nameStatus === "error") {
		statusText = "Could not load the automation name.";
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				{variant === "card" ? (
					<TimelineNotice
						// The note hosts a tooltip with the full IDs, so keyboard focus
						// must reach it.
						tabIndex={0}
						aria-label={`Automation run${suffix}`}
						className="cursor-default break-words focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
					>
						<span className="font-semibold">Automation run</span>
						{suffix}
					</TimelineNotice>
				) : (
					<Badge
						role="note"
						tabIndex={0}
						aria-label={`Automation run${suffix}`}
						size="sm"
						variant="outline"
						// The right padding moves to the text span, so its clip edge sits
						// at the badge border. See the name span below.
						className="min-w-0 max-w-full cursor-default pr-0 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
					>
						<span className="min-w-0 overflow-hidden whitespace-pre pr-1.5">
							Automation run
							{nameOrId && (
								<>
									{labelSeparator}
									{/* Only the name truncates: its max width reserves room for the
									    prefix and kind. Flex would split the copied text into lines. */}
									<span
										className={cn(
											"inline-block truncate align-bottom",
											kindSuffix
												? "max-w-[calc(100%-18.5ch)]"
												: "max-w-[calc(100%-11ch)]",
										)}
									>
										{nameOrId}
									</span>
									{kindSuffix}
								</>
							)}
						</span>
					</Badge>
				)}
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs break-words">
				<div>{statusText}</div>
				<div className="font-mono">Automation ID: {automationId}</div>
				{inputId && <div className="font-mono">Input ID: {inputId}</div>}
			</TooltipContent>
		</Tooltip>
	);
};
