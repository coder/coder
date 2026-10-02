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
	/** The automation name, its ID, or empty while loading. */
	name: string;
	/** " (<kind>)" when the name is known, otherwise empty. */
	kindSuffix: string;
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
		name = reference.name;
	} else if (showNameLoading) {
		name = "";
	}
	const kindSuffix = reference ? ` (${reference.kind})` : "";
	let statusText = "Automation name unavailable.";
	if (reference) {
		statusText = `Automation: ${name}${kindSuffix}`;
	} else if (showNameLoading) {
		statusText = "Loading automation name.";
	} else if (nameStatus === "error") {
		statusText = "Could not load the automation name.";
	}
	return {
		name,
		kindSuffix,
		text: `Automation run${name ? `${labelSeparator}${name}${kindSuffix}` : ""}`,
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
	const { name, kindSuffix, text, statusText } = formatAutomationLabel({
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
					"my-1 flex w-full flex-row items-start gap-3 rounded-lg border border-solid border-border-default bg-surface-secondary p-4 text-left text-sm font-normal text-content-primary",
					focusRing,
				)}
			>
				<InfoIcon
					aria-hidden="true"
					className="size-icon-sm mt-[3px] shrink-0 text-highlight-sky"
				/>
				<span className="min-w-0 flex-1 break-words">
					<span className="font-semibold">Automation run</span>
					{name && `${labelSeparator}${name}${kindSuffix}`}
				</span>
			</div>
		) : (
			<Badge
				role="note"
				tabIndex={0}
				aria-label={text}
				size="sm"
				variant="outline"
				// The right padding moves to the text span, so its clip edge sits
				// at the badge border. See the name span below.
				className={cn("min-w-0 max-w-full pr-0", focusRing)}
			>
				<span className="min-w-0 overflow-hidden whitespace-pre pr-1.5">
					Automation run
					{name && (
						<>
							{labelSeparator}
							{/*
							 * Only the name truncates, so the kind stays visible. Its
							 * max width reserves room for the text around it:
							 * "Automation run · " plus " (<kind>)" measure about
							 * 19.2ch in Geist, and the prefix alone about 11.6ch.
							 * The reserves are slightly smaller, so a name that fits
							 * never truncates. The small overflow lands in the right
							 * padding, which the overflow clip does not cover.
							 */}
							<span
								className={cn(
									"inline-block truncate align-bottom",
									kindSuffix
										? "max-w-[calc(100%-18.5ch)]"
										: "max-w-[calc(100%-11ch)]",
								)}
							>
								{name}
							</span>
							{kindSuffix}
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
