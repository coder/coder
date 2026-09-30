import { useFormik } from "formik";
import { useId } from "react";
import { useQuery } from "react-query";
import * as Yup from "yup";
import { chatModels } from "#/api/queries/chats";
import type {
	ChatAutomation,
	ChatAutomationTargetMode,
	ChatAutomationWhenBusy,
	CreateChatAutomationRequest,
	UpdateChatAutomationRequest,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { ModelSelector } from "#/modules/aiModels/ModelSelector";
import { getFormHelpers } from "#/utils/formUtils";
import { getPreferredTimezone } from "#/utils/timeZones";
import { resolveModelSelector } from "../../utils/modelOptions";
import { AutomationChatPicker } from "./AutomationChatPicker";
import { AutomationScheduleFields } from "./AutomationScheduleFields";

const NAME_MAX_LENGTH = 128;

// Field names match the API so getFormHelpers maps 400 validations onto them.
type AutomationFormValues = {
	name: string;
	prompt: string;
	schedule_cron: string;
	schedule_time_zone: string;
	target_mode: ChatAutomationTargetMode;
	target_chat_id: string;
	when_busy: ChatAutomationWhenBusy;
	new_chat_model_config_id: string;
	reasoning_effort: string;
};

const initialFormValues = (
	automation: ChatAutomation | undefined,
): AutomationFormValues => ({
	name: automation?.name ?? "",
	prompt: automation?.prompt ?? "",
	schedule_cron: automation ? (automation.schedule_cron ?? "") : "0 9 * * *",
	schedule_time_zone: automation
		? (automation.schedule_time_zone ?? "")
		: getPreferredTimezone(),
	target_mode: automation?.target_mode ?? "existing_chat",
	target_chat_id: automation?.target_chat_id ?? "",
	when_busy: automation?.when_busy ?? "skip",
	new_chat_model_config_id: automation?.new_chat_model_config_id ?? "",
	reasoning_effort: automation?.reasoning_effort ?? "",
});

const normalize = (values: AutomationFormValues): AutomationFormValues => ({
	...values,
	name: values.name.trim(),
	schedule_cron: values.schedule_cron.trim(),
});

const buildCreateRequest = (
	values: AutomationFormValues,
): CreateChatAutomationRequest => {
	const shared = {
		name: values.name,
		kind: "schedule",
		target_mode: values.target_mode,
		prompt: values.prompt,
		schedule_cron: values.schedule_cron,
		schedule_time_zone: values.schedule_time_zone,
	} as const;
	if (values.target_mode === "existing_chat") {
		return {
			...shared,
			target_chat_id: values.target_chat_id,
			when_busy: values.when_busy,
		};
	}
	return {
		...shared,
		new_chat_model_config_id: values.new_chat_model_config_id,
		...(values.reasoning_effort && {
			reasoning_effort: values.reasoning_effort,
		}),
	};
};

/** Sends only the changed fields that apply to the automation's kind and mode. */
const buildUpdateRequest = (
	automation: ChatAutomation,
	initial: AutomationFormValues,
	values: AutomationFormValues,
): UpdateChatAutomationRequest => {
	const changed = (key: keyof AutomationFormValues) =>
		values[key] !== initial[key];
	const isSchedule = automation.kind === "schedule";
	const isExistingChat = automation.target_mode === "existing_chat";
	return {
		...(changed("name") && { name: values.name }),
		...(changed("prompt") && { prompt: values.prompt }),
		...(isSchedule &&
			changed("schedule_cron") && { schedule_cron: values.schedule_cron }),
		...(isSchedule &&
			changed("schedule_time_zone") && {
				schedule_time_zone: values.schedule_time_zone,
			}),
		...(isExistingChat &&
			changed("target_chat_id") && { target_chat_id: values.target_chat_id }),
		...(isExistingChat &&
			changed("when_busy") && { when_busy: values.when_busy }),
		...(!isExistingChat &&
			changed("new_chat_model_config_id") && {
				new_chat_model_config_id: values.new_chat_model_config_id,
			}),
		...(!isExistingChat &&
			changed("reasoning_effort") && {
				reasoning_effort: values.reasoning_effort,
			}),
	};
};

type AutomationEditorDialogProps = {
	organizationId: string;
	/** Edits this automation; creates a schedule when unset. */
	automation?: ChatAutomation;
	currentUserId: string;
	error: unknown;
	isSubmitting: boolean;
	onCreate: (req: CreateChatAutomationRequest) => void;
	onUpdate: (req: UpdateChatAutomationRequest) => void;
	onClose: () => void;
};

export const AutomationEditorDialog: React.FC<AutomationEditorDialogProps> = ({
	organizationId,
	automation,
	currentUserId,
	error,
	isSubmitting,
	onCreate,
	onUpdate,
	onClose,
}) => {
	const isCreate = !automation;
	const isSchedule = !automation || automation.kind === "schedule";
	const chatPickerId = useId();
	const chatErrorId = useId();
	const whenBusyId = useId();
	const modelsQuery = useQuery(chatModels(organizationId));
	const { options: modelOptions } = resolveModelSelector(
		organizationId,
		modelsQuery,
	);

	const initialValues = initialFormValues(automation);
	const form = useFormik<AutomationFormValues>({
		initialValues,
		validationSchema: Yup.object({
			name: Yup.string()
				.trim()
				.required("Name is required.")
				.max(
					NAME_MAX_LENGTH,
					`Name must be at most ${NAME_MAX_LENGTH} characters.`,
				),
			prompt: Yup.string().trim().required("Prompt is required."),
			schedule_cron: isSchedule
				? Yup.string().trim().required("Cron expression is required.")
				: Yup.string(),
			target_chat_id: Yup.string().when("target_mode", {
				is: "existing_chat",
				then: (schema) => schema.required("Choose a chat."),
			}),
			new_chat_model_config_id: Yup.string().when("target_mode", {
				is: "new_chat",
				then: (schema) => schema.required("Choose a model."),
			}),
		}),
		onSubmit: (rawValues) => {
			const values = normalize(rawValues);
			if (!automation) {
				onCreate(buildCreateRequest(values));
				return;
			}
			const req = buildUpdateRequest(automation, initialValues, values);
			if (Object.keys(req).length === 0) {
				onClose();
				return;
			}
			onUpdate(req);
		},
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const chatField = getFieldHelpers("target_chat_id");
	const modelField = getFieldHelpers("new_chat_model_config_id");
	const isExistingChat = form.values.target_mode === "existing_chat";

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open) {
					onClose();
				}
			}}
		>
			<DialogContent className="flex max-h-[90vh] max-w-2xl flex-col gap-0 overflow-hidden p-0">
				<form
					className="flex min-h-0 flex-1 flex-col"
					onSubmit={form.handleSubmit}
				>
					<DialogHeader className="px-6 pt-6">
						<DialogTitle>
							{isCreate ? "New automation" : `Edit ${automation.name}`}
						</DialogTitle>
						<DialogDescription>
							{isCreate
								? "Send a prompt to an agent on a schedule."
								: "The trigger and target type cannot change after creation."}
						</DialogDescription>
					</DialogHeader>
					<div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 py-4">
						{Boolean(error) && (
							<ErrorAlert error={error} showDebugDetail={false} />
						)}
						<FormField
							field={getFieldHelpers("name", { maxLength: NAME_MAX_LENGTH })}
							label="Name"
							required
						/>
						<FormField
							field={getFieldHelpers("prompt")}
							label="Prompt"
							required
							control={(props) => (
								<Textarea
									{...props}
									{...form.getFieldProps("prompt")}
									rows={4}
								/>
							)}
						/>
						<section className="flex flex-col gap-4">
							<h3 className="m-0 text-sm font-medium text-content-primary">
								Trigger:{" "}
								<span className="font-normal text-content-secondary">
									{isSchedule ? "Schedule" : "Webhook"}
								</span>
							</h3>
							{isSchedule && (
								<AutomationScheduleFields
									organizationId={organizationId}
									isCreate={isCreate}
									cronField={getFieldHelpers("schedule_cron")}
									timeZoneField={getFieldHelpers("schedule_time_zone")}
									onCronChange={(cron) =>
										form.setFieldValue("schedule_cron", cron)
									}
									onTimeZoneChange={(timeZone) =>
										form.setFieldValue("schedule_time_zone", timeZone)
									}
								/>
							)}
						</section>
						<section className="flex flex-col gap-4">
							<h3 className="m-0 text-sm font-medium text-content-primary">
								Target
							</h3>
							<p className="m-0 text-xs text-content-secondary">
								Changes apply to the next run.
							</p>
							{isCreate ? (
								<RadioGroup
									aria-label="Target"
									value={form.values.target_mode}
									onValueChange={(value) => {
										if (value === "existing_chat" || value === "new_chat") {
											form.setFieldValue("target_mode", value);
										}
									}}
								>
									<Label className="flex items-center gap-2 font-normal">
										<RadioGroupItem value="existing_chat" />
										Existing chat
									</Label>
									<Label className="flex items-center gap-2 font-normal">
										<RadioGroupItem value="new_chat" />
										New chat each run
									</Label>
								</RadioGroup>
							) : (
								<p className="m-0 text-sm text-content-secondary">
									{isExistingChat ? "Existing chat" : "New chat each run"}
								</p>
							)}
							{isExistingChat ? (
								<div className="grid grid-cols-2 gap-4">
									<div className="flex flex-col gap-2">
										<Label htmlFor={chatPickerId}>Chat</Label>
										<AutomationChatPicker
											id={chatPickerId}
											value={form.values.target_chat_id}
											currentUserId={currentUserId}
											invalid={chatField.error}
											describedBy={chatField.error ? chatErrorId : undefined}
											onChange={(chatId) =>
												form.setFieldValue("target_chat_id", chatId)
											}
										/>
										{chatField.error && (
											<span
												id={chatErrorId}
												className="text-xs text-content-destructive"
											>
												{chatField.helperText}
											</span>
										)}
									</div>
									<div className="flex flex-col gap-2">
										<Label htmlFor={whenBusyId}>When busy</Label>
										<Select
											value={form.values.when_busy}
											onValueChange={(value) => {
												if (value === "skip" || value === "queue") {
													form.setFieldValue("when_busy", value);
												}
											}}
										>
											<SelectTrigger id={whenBusyId}>
												<SelectValue />
											</SelectTrigger>
											<SelectContent>
												<SelectItem value="skip">Skip the run</SelectItem>
												<SelectItem value="queue">Queue the prompt</SelectItem>
											</SelectContent>
										</Select>
									</div>
								</div>
							) : (
								<div className="flex flex-col gap-2">
									<span className="text-sm font-medium text-content-primary">
										Model
									</span>
									<ModelSelector
										className="w-fit"
										triggerAriaLabel="Model"
										options={modelOptions}
										value={form.values.new_chat_model_config_id}
										onValueChange={(modelId) => {
											form.setFieldValue("new_chat_model_config_id", modelId);
											form.setFieldValue("reasoning_effort", "");
										}}
										reasoningEffort={form.values.reasoning_effort}
										onReasoningEffortChange={(effort) =>
											form.setFieldValue("reasoning_effort", effort)
										}
									/>
									{modelsQuery.isError && (
										<span className="text-xs text-content-destructive">
											Could not load models.
										</span>
									)}
									{modelField.error && (
										<span className="text-xs text-content-destructive">
											{modelField.helperText}
										</span>
									)}
								</div>
							)}
						</section>
					</div>
					<DialogFooter className="border-0 border-t border-solid border-border-default px-6 py-4">
						<Button type="button" variant="outline" onClick={onClose}>
							Cancel
						</Button>
						<Button type="submit" disabled={isSubmitting}>
							<Spinner loading={isSubmitting} />
							Save
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
