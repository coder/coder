import { useFormik } from "formik";
import { useState } from "react";
import * as Yup from "yup";
import { getErrorMessage, isApiError } from "#/api/errors";
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
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FormField } from "#/components/FormField/FormField";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { useRestoreFocusOnClose } from "#/hooks/useRestoreFocusOnClose";
import { getFormHelpers } from "#/utils/formUtils";
import { getPreferredTimezone } from "#/utils/timeZones";
import { AutomationTargetField } from "./AutomationTargetField";
import { AutomationTriggerField } from "./AutomationTriggerField";

const NAME_MAX_LENGTH = 128;

// Labels for the server's field names, and the choice that hides a field,
// which makes the field's server error stale when that choice changes.
const SERVER_FIELDS: Record<
	string,
	{ label: string; shownFor?: "kind" | "target_mode" }
> = {
	name: { label: "Name" },
	prompt: { label: "Prompt" },
	schedule_cron: { label: "Cron expression", shownFor: "kind" },
	schedule_time_zone: { label: "Time zone", shownFor: "kind" },
	webhook_use: { label: "Use", shownFor: "kind" },
	target_chat_id: { label: "Chat", shownFor: "target_mode" },
	when_busy: { label: "When busy", shownFor: "target_mode" },
	new_chat_model_config_id: { label: "Model", shownFor: "target_mode" },
	reasoning_effort: { label: "Reasoning effort", shownFor: "target_mode" },
};

// Field names match the API so getFormHelpers maps 400 validations onto them.
export type AutomationFormValues = {
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
	/** Origin of the webhook publish endpoint. */
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
	const isReadOnly = Boolean(
		automation && automation.owner_id !== currentUserId,
	);
	// A user-picked When busy value survives trigger changes.
	const [whenBusyChosen, setWhenBusyChosen] = useState(false);
	const restoreFocus = useRestoreFocusOnClose();
	const [submittedValues, setSubmittedValues] =
		useState<AutomationFormValues>();

	// Frozen at open, so a list refetch that refreshes `automation` neither
	// resets the user's input nor changes the PATCH baseline.
	const [initialValues] = useState(() => initialFormValues(automation));
	const form = useFormik<AutomationFormValues>({
		initialValues,
		// A blur error would shift the fields below it between pointerdown and
		// pointerup, so the click on a radio below would not register.
		validateOnBlur: false,
		validationSchema: Yup.object({
			name: Yup.string()
				.trim()
				.required("Name is required.")
				// The server counts code points, so an emoji is one character.
				.test(
					"max-code-points",
					`Name must be at most ${NAME_MAX_LENGTH} characters.`,
					(value = "") => [...value].length <= NAME_MAX_LENGTH,
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
	// A server error applies while its field, and the choice that shows the
	// field, still hold the submitted values.
	const serverErrorApplies = (field: string) => {
		if (!submittedValues) {
			return false;
		}
		const shownFor = SERVER_FIELDS[field]?.shownFor;
		if (shownFor && submittedValues[shownFor] !== form.values[shownFor]) {
			return false;
		}
		const isFormField = (key: string): key is keyof AutomationFormValues =>
			key in submittedValues;
		return !isFormField(field) || submittedValues[field] === form.values[field];
	};
	const getFieldHelpers = (
		name: keyof AutomationFormValues,
		options?: { helperText?: React.ReactNode },
	) =>
		getFormHelpers(form, serverErrorApplies(name) ? error : undefined)(
			name,
			options,
		);
	const isSchedule = form.values.kind === "schedule";
	const isExistingChat = form.values.target_mode === "existing_chat";

	const renderedFields: readonly string[] = [
		"name",
		"prompt",
		...(isSchedule ? ["schedule_cron", "schedule_time_zone"] : []),
		...(isExistingChat
			? ["target_chat_id", "when_busy"]
			: ["new_chat_model_config_id", "reasoning_effort"]),
	];
	const apiError = isApiError(error) ? error.response.data : undefined;
	const alertValidations = (apiError?.validations ?? []).filter(
		(validation) =>
			!renderedFields.includes(validation.field) &&
			serverErrorApplies(validation.field),
	);
	const isPending = isSubmitting || isRotatingSecret;
	const showAlert =
		Boolean(error) &&
		(!apiError?.validations?.length || alertValidations.length > 0);

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open && !isPending) {
					onClose();
				}
			}}
		>
			<DialogContent
				className="flex max-w-2xl flex-col gap-0 overflow-hidden p-0"
				onCloseAutoFocus={restoreFocus}
			>
				<form
					className="flex min-h-0 flex-1 flex-col"
					onSubmit={form.handleSubmit}
					noValidate
				>
					<DialogHeader className="px-6 pt-6">
						<DialogTitle>
							{isCreate
								? "New automation"
								: isReadOnly
									? "View automation"
									: "Edit automation"}
						</DialogTitle>
						<DialogDescription>
							{isCreate
								? "Create a schedule or webhook that sends a prompt to an agent."
								: isReadOnly
									? "Only the owner of this automation can change it."
									: "Changes apply to the next run."}
						</DialogDescription>
					</DialogHeader>
					{/* Browser and Radix constraints on this layout:
					    - Chrome does not scroll a flex-sized fieldset, so the div scrolls.
					    - The focus trap wraps Tab with preventScroll, so onFocus scrolls
					      the wrapped-to field into view.
					    - Radix Select opens on pointerdown, which browsers still send to
					      fieldset-disabled buttons, so they get pointer-events: none. */}
					<div
						className="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-4"
						onFocus={(event) => {
							if (event.target instanceof HTMLElement) {
								// The parent first keeps a field's label in view too.
								event.target.parentElement?.scrollIntoView({
									block: "nearest",
								});
								event.target.scrollIntoView({ block: "nearest" });
							}
						}}
					>
						<fieldset
							disabled={isSubmitting || isReadOnly}
							className="m-0 flex min-w-0 flex-col gap-6 border-0 p-0 [&_button:disabled]:pointer-events-none"
						>
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
													{SERVER_FIELDS[validation.field]
														? `${SERVER_FIELDS[validation.field].label}: ${validation.detail}`
														: validation.detail}
												</span>
											))}
										</AlertDescription>
									)}
								</Alert>
							)}
							<FormField
								field={getFieldHelpers("name")}
								label="Name"
								required
							/>
							<FormField
								field={getFieldHelpers("prompt", {
									helperText: isSchedule
										? "Sent as the message for every run."
										: "Sent as the message for every run. The webhook request body is attached below it as untrusted event data. Say what to check, when to act, and when to do nothing.",
								})}
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
							<AutomationTriggerField
								isCreate={isCreate}
								kind={form.values.kind}
								onKindChange={(kind) => {
									if (!whenBusyChosen) {
										form.setFieldValue("when_busy", defaultWhenBusy(kind));
									}
									form.setFieldValue("kind", kind);
								}}
								scheduleFieldsProps={{
									organizationId,
									cronField: getFieldHelpers("schedule_cron"),
									timeZoneField: getFieldHelpers("schedule_time_zone"),
									onCronChange: (cron) =>
										form.setFieldValue("schedule_cron", cron),
									onTimeZoneChange: (timeZone) =>
										form.setFieldValue("schedule_time_zone", timeZone),
								}}
								webhookFieldsProps={{
									automation,
									origin,
									webhookUse: form.values.webhook_use,
									onWebhookUseChange: (webhookUse) =>
										form.setFieldValue("webhook_use", webhookUse),
									rotateSecretError,
									isRotatingSecret,
									isSubmitting,
									canRotateSecret: !isReadOnly,
									onRotateSecret,
								}}
							/>
							<AutomationTargetField
								organizationId={organizationId}
								isCreate={isCreate}
								form={form}
								getFieldHelpers={getFieldHelpers}
								onWhenBusyChange={() => setWhenBusyChosen(true)}
							/>
						</fieldset>
					</div>
					<DialogFooter className="border-0 border-t border-solid border-border-default px-6 py-4">
						<Button
							type="button"
							variant="outline"
							disabled={isPending}
							onClick={onClose}
						>
							{isReadOnly ? "Close" : "Cancel"}
						</Button>
						{!isReadOnly && (
							<Button type="submit" disabled={isPending}>
								<Spinner loading={isSubmitting} />
								Save
							</Button>
						)}
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
