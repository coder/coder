import { useId } from "react";
import type { ChatAutomationKind } from "#/api/typesGenerated";
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import { AutomationScheduleFields } from "./AutomationScheduleFields";
import { AutomationWebhookFields } from "./AutomationWebhookFields";
import { RadioOption } from "./RadioOption";

type AutomationTriggerFieldProps = {
	isCreate: boolean;
	kind: ChatAutomationKind;
	onKindChange: (kind: ChatAutomationKind) => void;
	scheduleFieldsProps: React.ComponentProps<typeof AutomationScheduleFields>;
	webhookFieldsProps: React.ComponentProps<typeof AutomationWebhookFields>;
};

/** The trigger kind and the fields of the chosen kind. */
export const AutomationTriggerField: React.FC<AutomationTriggerFieldProps> = ({
	isCreate,
	kind,
	onKindChange,
	scheduleFieldsProps,
	webhookFieldsProps,
}) => {
	const triggerLabelId = useId();
	const isSchedule = kind === "schedule";
	return (
		<section className="flex flex-col gap-4">
			{isCreate ? (
				<>
					<h3
						id={triggerLabelId}
						className="m-0 text-sm font-medium text-content-primary"
					>
						Trigger
					</h3>
					<RadioGroup
						aria-labelledby={triggerLabelId}
						value={kind}
						onValueChange={(value) => {
							if (value === "schedule" || value === "webhook") {
								onKindChange(value);
							}
						}}
					>
						<RadioOption value="schedule" label="Schedule" />
						<RadioOption value="webhook" label="Webhook" />
					</RadioGroup>
				</>
			) : (
				<h3 className="m-0 text-sm font-medium text-content-primary">
					Trigger:{" "}
					<span className="font-normal text-content-secondary">
						{isSchedule ? "Schedule" : "Webhook"}
					</span>
				</h3>
			)}
			{isSchedule ? (
				<AutomationScheduleFields {...scheduleFieldsProps} />
			) : (
				<AutomationWebhookFields {...webhookFieldsProps} />
			)}
		</section>
	);
};
