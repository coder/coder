import { useId } from "react";
import type { ChatAutomationKind } from "#/api/typesGenerated";
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import { AutomationScheduleFields } from "./AutomationScheduleFields";
import { AutomationWebhookFields } from "./AutomationWebhookFields";
import { FixedAtCreationBadge } from "./FixedAtCreationBadge";
import { RadioOption } from "./RadioOption";

type AutomationTriggerFieldProps = {
	isCreate: boolean;
	kind: ChatAutomationKind;
	onKindChange: (kind: ChatAutomationKind) => void;
	scheduleFieldsProps: React.ComponentProps<typeof AutomationScheduleFields>;
	webhookFieldsProps: React.ComponentProps<typeof AutomationWebhookFields>;
};

export const AutomationTriggerField: React.FC<AutomationTriggerFieldProps> = ({
	isCreate,
	kind,
	onKindChange,
	scheduleFieldsProps,
	webhookFieldsProps,
}) => {
	const triggerLabelId = useId();
	const triggerDescriptionId = useId();
	return (
		<section className="flex flex-col gap-4">
			<div className="flex items-start justify-between gap-2">
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
				{!isCreate && <FixedAtCreationBadge />}
			</div>
			<RadioGroup
				aria-labelledby={triggerLabelId}
				aria-describedby={triggerDescriptionId}
				value={kind}
				disabled={!isCreate}
				onValueChange={(value) => {
					if (value === "schedule" || value === "webhook") {
						onKindChange(value);
					}
				}}
			>
				<RadioOption value="schedule" label="Schedule" />
				<RadioOption value="webhook" label="Webhook" />
			</RadioGroup>
			{kind === "schedule" ? (
				<AutomationScheduleFields {...scheduleFieldsProps} />
			) : (
				<AutomationWebhookFields {...webhookFieldsProps} />
			)}
		</section>
	);
};
