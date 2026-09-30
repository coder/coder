import { useFormik } from "formik";
import { useId, useRef, useState } from "react";
import { useQuery } from "react-query";
import * as Yup from "yup";
import { getErrorMessage, isApiError } from "#/api/errors";
import { chatModels } from "#/api/queries/chats";
import type {
	ChatAutomation,
	ChatAutomationKind,
	ChatAutomationTargetMode,
	ChatAutomationWebhookUse,
	ChatAutomationWhenBusy,
	CreateChatAutomationRequest,
	UpdateChatAutomationRequest,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
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
import { SelectItem } from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { ModelSelector } from "#/modules/aiModels/ModelSelector";
import { getFormHelpers } from "#/utils/formUtils";
import { formatDate } from "#/utils/time";
import { getPreferredTimezone } from "#/utils/timeZones";
import { resolveModelSelector } from "../../utils/modelOptions";
import { pickReasoningEffort } from "../../utils/reasoningEffort";
import { AutomationChatPicker } from "./AutomationChatPicker";
import { AutomationScheduleFields } from "./AutomationScheduleFields";
import { webhookEventsUrl } from "./AutomationWebhookSecretDialog";

const NAME_MAX_LENGTH = 128;

// Field names match the API so getFormHelpers maps 400 validations onto them.
type AutomationFormValues = {
	name: string;
	prompt: string;
	kind: ChatAutomationKind;
	webhook_use: ChatAutomationWebhookUse;
	schedule_cron: string;
	schedule_time_zone: string;
	target_mode: ChatAutomationTargetMode;
	target_chat_id: string;
	when_busy: ChatAutomationWhenBusy;
	new_chat_model_config_id: string;
	reasoning_effort: string;
};

// Matches the server defaults: webhooks queue, schedules skip.
const defaultWhenBusy = (kind: ChatAutomationKind): ChatAutomationWhenBusy =>
	kind === "webhook" ? "queue" : "skip";

const initialFormValues = (
	automation: ChatAutomation | undefined,
): AutomationFormValues => ({
	name: automation?.name ?? "",
	prompt: automation?.prompt ?? "",
	kind: automation?.kind ?? "schedule",
	webhook_use: automation?.webhook_use ?? "multi",
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
		target_mode: values.target_mode,
		prompt: values.prompt,
		kind: values.kind,
		...(values.kind === "schedule"
			? {
					schedule_cron: values.schedule_cron,
					schedule_time_zone: values.schedule_time_zone,
				}
			: { webhook_use: values.webhook_use }),
	};
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

/** Builds a PATCH body with only the changed fields that apply to the automation's kind and target mode. */
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
	/** Edits this automation; creates a new one when unset. */
	automation?: ChatAutomation;
	currentUserId: string;
	/** Origin of the webhook publish endpoint, usually `window.location.origin`. */
	origin: string;
	error: unknown;
	isSubmitting: boolean;
	rotateSecretError: unknown;
	isRotatingSecret: boolean;
	onCreate: (req: CreateChatAutomationRequest) => void;
	onUpdate: (req: UpdateChatAutomationRequest) => void;
	/** Receives the Rotate secret button so focus can return to it later. */
	onRotateSecret: (rotateButton: HTMLButtonElement | null) => void;
	onClose: () => void;
};

export const AutomationEditorDialog: React.FC<AutomationEditorDialogProps> = ({
	organizationId,
	automation,
	currentUserId,
	origin,
	error,
	isSubmitting,
	rotateSecretError,
	isRotatingSecret,
	onCreate,
	onUpdate,
	onRotateSecret,
	onClose,
}) => {
	const isCreate = !automation;
	const existingChatId = useId();
	const newChatId = useId();
	const scheduleKindId = useId();
	const webhookKindId = useId();
	const multiUseId = useId();
	const singleUseId = useId();
	const rotateButtonRef = useRef<HTMLButtonElement>(null);
	const [confirmingRotate, setConfirmingRotate] = useState(false);
	// A user-picked When busy value survives trigger changes.
	const [whenBusyChosen, setWhenBusyChosen] = useState(false);
	// Radix returns focus to a DialogTrigger on close; this dialog has none.
	const [opener] = useState(() =>
		document.activeElement instanceof HTMLElement
			? document.activeElement
			: null,
	);
	const [submittedValues, setSubmittedValues] =
		useState<AutomationFormValues>();
	const modelsQuery = useQuery(chatModels(organizationId));
	const { options: modelOptions } = resolveModelSelector(
		organizationId,
		modelsQuery,
	);

	const initialValues = initialFormValues(automation);
	const form = useFormik<AutomationFormValues>({
		initialValues,
		// A blur error would shift the fields below it between pointerdown and
		// pointerup, so the click on a radio below would not register.
		validateOnBlur: false,
		validationSchema: Yup.object({
			name: Yup.string()
				.trim()
				.required("Name is required.")
				.max(
					NAME_MAX_LENGTH,
					`Name must be at most ${NAME_MAX_LENGTH} characters.`,
				),
			prompt: Yup.string().trim().required("Prompt is required."),
			schedule_cron: Yup.string().when("kind", {
				is: "schedule",
				then: (schema) =>
					schema.trim().required("Cron expression is required."),
			}),
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
			setSubmittedValues(rawValues);
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
	const getFieldHelpers = (
		name: keyof AutomationFormValues,
		options?: { maxLength?: number },
	) =>
		getFormHelpers(
			form,
			submittedValues?.[name] === form.values[name] ? error : undefined,
		)(name, options);
	const modelField = getFieldHelpers("new_chat_model_config_id");
	const isSchedule = form.values.kind === "schedule";
	const handleKindChange = (kind: ChatAutomationKind) => {
		if (!whenBusyChosen) {
			form.setFieldValue("when_busy", defaultWhenBusy(kind));
		}
		form.setFieldValue("kind", kind);
	};
	const isExistingChat = form.values.target_mode === "existing_chat";
	const selectedModel = modelOptions.find(
		(option) => option.id === form.values.new_chat_model_config_id,
	);

	const renderedFields: readonly string[] = [
		"name",
		"prompt",
		...(isSchedule ? ["schedule_cron", "schedule_time_zone"] : []),
		...(isExistingChat
			? ["target_chat_id", "when_busy"]
			: ["new_chat_model_config_id"]),
	];
	const apiError = isApiError(error) ? error.response.data : undefined;
	const alertValidations = (apiError?.validations ?? []).filter(
		(validation) => !renderedFields.includes(validation.field),
	);
	const rotateApiError = isApiError(rotateSecretError)
		? rotateSecretError.response.data
		: undefined;
	const showAlert =
		Boolean(error) &&
		(!apiError?.validations?.length || alertValidations.length > 0);

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open) {
					onClose();
				}
			}}
		>
			<DialogContent
				className="flex max-w-2xl flex-col gap-0 overflow-hidden p-0"
				onCloseAutoFocus={(event) => {
					if (opener?.isConnected) {
						event.preventDefault();
						opener.focus();
					}
				}}
			>
				<form
					className="flex min-h-0 flex-1 flex-col"
					onSubmit={form.handleSubmit}
					noValidate
				>
					<DialogHeader className="px-6 pt-6">
						<DialogTitle>
							{isCreate ? "New automation" : `Edit ${automation.name}`}
						</DialogTitle>
						<DialogDescription>
							{isCreate
								? "Create a schedule or webhook that sends a prompt to an agent."
								: "The trigger and target type cannot change after creation."}
						</DialogDescription>
					</DialogHeader>
					<div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 py-4">
						{automation && automation.owner_id !== currentUserId && (
							<p className="m-0 text-sm text-content-secondary">
								Only the owner of this automation can save changes.
							</p>
						)}
						{showAlert && (
							<Alert severity="error" prominent>
								<AlertTitle>
									{getErrorMessage(error, "Could not save the automation.")}
								</AlertTitle>
								{(apiError?.detail || alertValidations.length > 0) && (
									<AlertDescription>
										{apiError?.detail}
										{alertValidations.map((validation) => (
											<span key={validation.field} className="block">
												{validation.field}: {validation.detail}
											</span>
										))}
									</AlertDescription>
								)}
							</Alert>
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
							{isCreate ? (
								<>
									<h3 className="m-0 text-sm font-medium text-content-primary">
										Trigger
									</h3>
									<RadioGroup
										aria-label="Trigger"
										value={form.values.kind}
										onValueChange={(value) => {
											if (value === "schedule" || value === "webhook") {
												handleKindChange(value);
											}
										}}
									>
										<div className="flex items-center gap-2">
											<RadioGroupItem id={scheduleKindId} value="schedule" />
											<Label htmlFor={scheduleKindId} className="font-normal">
												Schedule
											</Label>
										</div>
										<div className="flex items-center gap-2">
											<RadioGroupItem id={webhookKindId} value="webhook" />
											<Label htmlFor={webhookKindId} className="font-normal">
												Webhook
											</Label>
										</div>
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
							{!isSchedule && isCreate && (
								<div className="flex flex-col gap-2">
									<span className="text-sm font-medium text-content-primary">
										Use
									</span>
									<RadioGroup
										aria-label="Use"
										value={form.values.webhook_use}
										onValueChange={(value) => {
											if (value === "single" || value === "multi") {
												form.setFieldValue("webhook_use", value);
											}
										}}
									>
										<div className="flex items-center gap-2">
											<RadioGroupItem id={singleUseId} value="single" />
											<Label htmlFor={singleUseId} className="font-normal">
												Single use
											</Label>
										</div>
										<div className="flex items-center gap-2">
											<RadioGroupItem id={multiUseId} value="multi" />
											<Label htmlFor={multiUseId} className="font-normal">
												Multi use
											</Label>
										</div>
									</RadioGroup>
									<span className="text-xs text-content-secondary">
										A single-use webhook accepts one event.
									</span>
								</div>
							)}
							{automation?.kind === "webhook" && (
								<>
									<p className="m-0 text-sm text-content-secondary">
										{automation.webhook_use === "single"
											? "Single use"
											: "Multi use"}
									</p>
									{automation.webhook_consumed_at && (
										<p className="m-0 text-sm text-content-secondary">
											Used on{" "}
											{formatDate(new Date(automation.webhook_consumed_at), {
												locale: "en-US",
												timeZoneName: "short",
											})}
										</p>
									)}
									<div className="flex flex-col gap-2">
										<span className="text-sm font-medium text-content-primary">
											Publish endpoint
										</span>
										<CodeExample
											secret={false}
											code={webhookEventsUrl(origin, automation.id)}
											copyLabel="Copy endpoint"
										/>
									</div>
									{/* Next to the button so it stays in view in a scrolled form. */}
									{Boolean(rotateSecretError) && (
										<Alert severity="error" prominent>
											<AlertTitle>
												{getErrorMessage(
													rotateSecretError,
													"Could not rotate the webhook secret.",
												)}
											</AlertTitle>
											{rotateApiError?.detail && (
												<AlertDescription>
													{rotateApiError.detail}
												</AlertDescription>
											)}
										</Alert>
									)}
									{/* A consumed single-use webhook rejects every event, so a new secret would be useless. */}
									{!(
										automation.webhook_use === "single" &&
										automation.webhook_consumed_at
									) && (
										<Button
											ref={rotateButtonRef}
											type="button"
											variant="outline"
											size="sm"
											className="w-fit"
											disabled={isRotatingSecret}
											onClick={() => setConfirmingRotate(true)}
										>
											<Spinner loading={isRotatingSecret} />
											Rotate secret
										</Button>
									)}
								</>
							)}
						</section>
						<section className="flex flex-col gap-4">
							<h3 className="m-0 text-sm font-medium text-content-primary">
								Target
							</h3>
							{!isCreate && (
								<p className="m-0 text-sm text-content-secondary">
									Changes apply to the next run.
								</p>
							)}
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
									<div className="flex items-center gap-2">
										<RadioGroupItem id={existingChatId} value="existing_chat" />
										<Label htmlFor={existingChatId} className="font-normal">
											Existing chat
										</Label>
									</div>
									<div className="flex items-center gap-2">
										<RadioGroupItem id={newChatId} value="new_chat" />
										<Label htmlFor={newChatId} className="font-normal">
											New chat each run
										</Label>
									</div>
								</RadioGroup>
							) : (
								<p className="m-0 text-sm text-content-secondary">
									{isExistingChat ? "Existing chat" : "New chat each run"}
								</p>
							)}
							{isExistingChat ? (
								<div className="grid grid-cols-2 gap-4">
									<FormField
										field={getFieldHelpers("target_chat_id")}
										label="Chat"
										control={(props) => (
											<AutomationChatPicker
												{...props}
												value={form.values.target_chat_id}
												onChange={(chatId) =>
													form.setFieldValue("target_chat_id", chatId)
												}
											/>
										)}
									/>
									<SelectField
										field={getFieldHelpers("when_busy")}
										label="When busy"
										onValueChange={(value) => {
											if (value === "skip" || value === "queue") {
												setWhenBusyChosen(true);
												form.setFieldValue("when_busy", value);
											}
										}}
									>
										<SelectItem value="skip">Skip the run</SelectItem>
										<SelectItem value="queue">Queue the prompt</SelectItem>
									</SelectField>
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
										reasoningEffort={
											selectedModel
												? pickReasoningEffort(
														form.values.reasoning_effort,
														selectedModel.reasoningEfforts ?? [],
														selectedModel.reasoningEffortDefault,
													)
												: form.values.reasoning_effort
										}
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
				<ConfirmDialog
					open={confirmingRotate}
					type="delete"
					title="Rotate the webhook secret?"
					description="The current secret stops working immediately."
					confirmText="Rotate secret"
					onClose={() => setConfirmingRotate(false)}
					onConfirm={() => {
						setConfirmingRotate(false);
						onRotateSecret(rotateButtonRef.current);
					}}
					onCloseAutoFocus={(event) => {
						event.preventDefault();
						rotateButtonRef.current?.focus();
					}}
				/>
			</DialogContent>
		</Dialog>
	);
};
