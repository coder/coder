import { useId } from "react";
import type { ChatAutomationKind } from "#/api/typesGenerated";
import { AutomationScheduleFields } from "./AutomationScheduleFields";
import { AutomationWebhookFields } from "./AutomationWebhookFields";
import { FixedAtCreationBadge } from "./FixedAtCreationBadge";
import { SegmentedRadios } from "./RadioCards";

const kindOptions = [
	{ value: "schedule", label: "Schedule" },
	{ value: "webhook", label: "Webhook" },
] as const;

type AutomationTriggerFieldProps = {
	isCreate: boolean;
	kind: ChatAutomationKind;
	onKindChange: (kind: ChatAutomationKind) => void;
	scheduleFieldsProps: React.ComponentProps<typeof AutomationScheduleFields>;
	webhookFieldsProps: React.ComponentProps<typeof AutomationWebhookFields>;
};

/**
 * The trigger section: a new automation picks its kind in the header, and a
 * saved one shows its fields with a Fixed at creation badge.
 */
export const AutomationTriggerField: React.FC<AutomationTriggerFieldProps> = ({
	isCreate,
	kind,
	onKindChange,
	scheduleFieldsProps,
	webhookFieldsProps,
}) => {
	const triggerLabelId = useId();
	const triggerDescriptionId = useId();
	const fixedId = useId();
	return (
		<section
			aria-labelledby={triggerLabelId}
			aria-describedby={
				isCreate ? triggerDescriptionId : `${triggerDescriptionId} ${fixedId}`
			}
			className="flex flex-col gap-3"
		>
			<div className="flex flex-wrap items-end justify-between gap-2">
				<div className="flex flex-col gap-1">
					<h3
						id={triggerLabelId}
						className="m-0 text-sm font-medium text-content-primary"
					>
						Trigger
					</h3>
					<p
						id={triggerDescriptionId}
						className="m-0 text-xs text-content-secondary"
					>
						What starts a run.
					</p>
				</div>
				{isCreate ? (
					<SegmentedRadios
						aria-labelledby={triggerLabelId}
						aria-describedby={triggerDescriptionId}
						value={kind}
						options={kindOptions}
						onValueChange={onKindChange}
					/>
				) : (
					<FixedAtCreationBadge
						id={fixedId}
						choice={
							kind === "schedule" ? "Schedule trigger" : "Webhook trigger"
						}
					/>
				)}
			</div>
			<div className="flex flex-col gap-4 rounded-lg border border-solid border-border p-4">
				{kind === "schedule" ? (
					<AutomationScheduleFields {...scheduleFieldsProps} />
				) : (
					<AutomationWebhookFields {...webhookFieldsProps} />
				)}
			</div>
		</section>
	);
};
