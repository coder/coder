import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import type {
	ChatAutomationReferenceInfo,
	ChatAutomationReferenceMap,
} from "#/api/queries/chatAutomations";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export type ChatAutomationNames = {
	names: ChatAutomationReferenceMap;
	// State of the chat's automation references: while "loading" (including
	// refetches) a missing name may still resolve, and "error" means the
	// request failed.
	status: "loading" | "error" | "settled";
};

const labelSeparator = " · ";

type AutomationLabelText = {
	/** The automation name and kind, its ID, or empty while loading. */
	name: string;
	/** The whole visible label as one line. */
	text: string;
	statusText: string;
};

/**
 * Formats an automation origin label as one line:
 * "Automation run · <name> (<kind>)". Without a name it shows the
 * automation ID, and while the name may still load it shows neither, so the
 * label does not flash the UUID. The input ID appears only in the tooltip.
 */
const formatAutomationLabel = ({
	automationId,
	reference,
	nameStatus,
}: {
	automationId: string;
	reference?: ChatAutomationReferenceInfo;
	nameStatus: ChatAutomationNames["status"];
}): AutomationLabelText => {
	const showNameLoading = !reference && nameStatus === "loading";
	let name = automationId;
	if (reference) {
		name = `${reference.name} (${reference.kind})`;
	} else if (showNameLoading) {
		name = "";
	}
	let statusText = "Automation name unavailable.";
	if (reference) {
		statusText = `Automation: ${name}`;
	} else if (showNameLoading) {
		statusText = "Loading automation name.";
	} else if (nameStatus === "error") {
		statusText = "Could not load the automation name.";
	}
	return {
		name,
		text: `Automation run${name ? `${labelSeparator}${name}` : ""}`,
		statusText,
	};
};

type AutomationLabelProps = {
	automationId: string;
	inputId?: string;
	reference?: ChatAutomationReferenceInfo;
	nameStatus: ChatAutomationNames["status"];
	/** "card" spans the timeline row; "badge" fits the queued list. */
	variant: "card" | "badge";
};

export const AutomationLabel: React.FC<AutomationLabelProps> = ({
	automationId,
	inputId,
	reference,
	nameStatus,
	variant,
}) => {
	const { name, text, statusText } = formatAutomationLabel({
		automationId,
		reference,
		nameStatus,
	});
	// The label is a focusable note rather than a button: it only hosts the
	// tooltip, which opens on hover and on keyboard focus. Its text is one
	// inline run in both variants, so selecting and copying it yields a
	// single line.
	const focusRing =
		"cursor-default focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link";
	const trigger =
		variant === "card" ? (
			<div
				role="note"
				// biome-ignore lint/a11y/noNoninteractiveTabindex: the note hosts a tooltip with the full IDs, so keyboard focus must reach it.
				tabIndex={0}
				aria-label={text}
				className={cn(
					"my-1 flex w-full flex-row items-start gap-3 rounded-lg border border-solid border-border-default bg-surface-secondary p-4 text-left text-sm text-content-primary",
					focusRing,
				)}
			>
				<InfoIcon
					aria-hidden="true"
					className="size-icon-sm mt-[3px] shrink-0 text-highlight-sky"
				/>
				<span className="min-w-0 flex-1 break-words">
					<span className="font-semibold">Automation run</span>
					{name && `${labelSeparator}${name}`}
				</span>
			</div>
		) : (
			<Badge
				role="note"
				tabIndex={0}
				aria-label={text}
				size="sm"
				variant="outline"
				className={cn("min-w-0 max-w-full", focusRing)}
			>
				<span className="min-w-0 overflow-hidden whitespace-pre">
					Automation run
					{name && (
						<>
							{labelSeparator}
							<span className="inline-block max-w-full truncate align-bottom">
								{name}
							</span>
						</>
					)}
				</span>
			</Badge>
		);
	return (
		<Tooltip>
			<TooltipTrigger asChild>{trigger}</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs break-words">
				<div>{statusText}</div>
				<div className="font-mono">Automation ID: {automationId}</div>
				{inputId && <div className="font-mono">Input ID: {inputId}</div>}
			</TooltipContent>
		</Tooltip>
	);
};
