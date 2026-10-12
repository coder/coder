import { useFormik } from "formik";
import { TriangleAlertIcon } from "lucide-react";
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
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { FormField } from "#/components/FormField/FormField";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { getFormHelpers } from "#/utils/formUtils";
import { getPreferredTimezone } from "#/utils/timeZones";
import { NIL_UUID } from "../../utils/modelOptions";
import { NO_PROJECT } from "./AutomationProjectField";
import { scheduleInactiveNotice } from "./AutomationStatus";
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
	project_id: { label: "Project", shownFor: "target_mode" },
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
	project_id: string;
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
	project_id: automation?.project_id ?? NO_PROJECT,
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
		...(values.project_id !== NO_PROJECT && { project_id: values.project_id }),
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
		...(!isExistingChat &&
			changed("project_id") && {
				project_id:
					values.project_id === NO_PROJECT ? NIL_UUID : values.project_id,
			}),
	};
};

/** Why leaving the page loses a one-time webhook secret. */
export type SecretGuard = "pending" | "shown";

const leavePromptCopy: Record<
	SecretGuard | "unsaved",
	{ title: string; description: string; confirmText: string }
> = {
	shown: {
		title: "Leave without copying the secret?",
		description:
			"The webhook secret is shown only once. If you leave now, you must rotate it to get a new one.",
		confirmText: "Leave",
	},
	pending: {
		title: "Leave before the secret arrives?",
		description:
			"The webhook secret is shown only once. If you leave now, you must rotate it to get a new one.",
		confirmText: "Leave",
	},
	unsaved: {
		title: "Discard unsaved changes?",
		description: "Your changes to this automation are not saved.",
		confirmText: "Discard",
	},
};

type AutomationEditorFormProps = {
	organizationId: string;
	/** Edits this automation; creates a new one when unset. */
	automation?: ChatAutomation;
	/** Shows the Project field for new chat targets. */
	projectsEnabled: boolean;
	/** Origin of the webhook publish endpoint. */
	origin: string;
	/** The name of the user whose permissions runs use. */
	runsAsName: string;
	error: unknown;
	isSubmitting: boolean;
	rotateSecretError: unknown;
	isRotatingSecret: boolean;
	/** Creates the automation, then turns it off unless `enable` is true. */
	onCreate: (req: CreateChatAutomationRequest, enable: boolean) => void;
	onUpdate: (req: UpdateChatAutomationRequest) => void;
	/** Receives the Rotate secret button so focus can return to it later. */
	onRotateSecret: (rotateButton: HTMLButtonElement | null) => void;
	onCancel: () => void;
	/** Set while leaving the page would lose a one-time webhook secret. */
	secretGuard?: SecretGuard;
	/** True once the page is about to leave on purpose, after a save. */
	allowLeave: boolean;
};

export const AutomationEditorForm: React.FC<AutomationEditorFormProps> = ({
	organizationId,
	automation,
	projectsEnabled,
	origin,
	runsAsName,
	error,
	isSubmitting,
	rotateSecretError,
	isRotatingSecret,
	onCreate,
	onUpdate,
	onRotateSecret,
	onCancel,
	secretGuard,
	allowLeave,
}) => {
	const isCreate = !automation;
	// A user-picked When busy value survives trigger changes.
	const [whenBusyChosen, setWhenBusyChosen] = useState(false);
	const [submittedValues, setSubmittedValues] =
		useState<AutomationFormValues>();
	// Which create button submitted the form.
	const [enableOnCreate, setEnableOnCreate] = useState(true);

	// Frozen at mount, so a list refetch that refreshes `automation` neither
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
			const values = {
				...normalize(rawValues),
				// The field unmounts if the experiment turns off while the
				// page is open; never send its hidden value.
				...(!projectsEnabled && { project_id: initialValues.project_id }),
			};
			if (!automation) {
				onCreate(buildCreateRequest(values), enableOnCreate);
				return;
			}
			const req = buildUpdateRequest(automation, initialValues, values);
			if (Object.keys(req).length === 0) {
				onCancel();
				return;
			}
			onUpdate(req);
		},
	});
	const leaveReason =
		secretGuard ?? (form.dirty && !allowLeave ? "unsaved" : undefined);
	const leavePrompt = useUnsavedChangesPrompt(
		leaveReason !== undefined,
		leaveReason,
	);
	const prompt = leaveReason ? leavePromptCopy[leaveReason] : undefined;

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
	) => {
		const serverError = serverErrorApplies(name) ? error : undefined;
		const helpers = getFormHelpers(form, serverError)(name, options);
		// Formik validates every field on each change, so an untouched empty
		// Name would turn red as soon as another field changes. Client
		// errors wait for the first Save; server errors show at once.
		if (form.submitCount === 0 && !serverError) {
			return { ...helpers, error: false, helperText: options?.helperText };
		}
		return helpers;
	};
	const isSchedule = form.values.kind === "schedule";
	const isExistingChat = form.values.target_mode === "existing_chat";
	const showProjectField = projectsEnabled && !isExistingChat;

	const renderedFields: readonly string[] = [
		"name",
		"prompt",
		...(isSchedule ? ["schedule_cron", "schedule_time_zone"] : []),
		...(isExistingChat
			? ["target_chat_id", "when_busy"]
			: ["new_chat_model_config_id", "reasoning_effort"]),
		...(showProjectField ? ["project_id"] : []),
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
		<form
			aria-label={isCreate ? "New automation" : "Edit automation"}
			className="flex flex-col gap-8"
			onSubmit={form.handleSubmit}
			noValidate
		>
			{/* Radix Select opens on pointerdown, which browsers still send to
			    fieldset-disabled buttons, so they get pointer-events: none. */}
			<fieldset
				disabled={isSubmitting}
				className="m-0 flex min-w-0 flex-col gap-8 border-0 p-0 [&_button:disabled]:pointer-events-none"
			>
				<FormField
					field={getFieldHelpers("name")}
					label="Name"
					required
					autoFocus={isCreate}
				/>
				<FormField
					field={getFieldHelpers("prompt", {
						helperText: [
							isSchedule
								? "Sent as the message for every run."
								: "Sent as the message for every run. The webhook request body is attached below it as untrusted event data. Say what to check, when to act, and when to do nothing.",
							// Queued messages keep the prompt they were sent with.
							!isCreate &&
								"Changes apply to future runs. Messages already queued keep the previous prompt.",
						]
							.filter(Boolean)
							.join(" "),
					})}
					label="Prompt"
					required
					control={(props) => (
						<Textarea {...props} {...form.getFieldProps("prompt")} rows={4} />
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
						onCronChange: (cron) => form.setFieldValue("schedule_cron", cron),
						onTimeZoneChange: (timeZone) =>
							form.setFieldValue("schedule_time_zone", timeZone),
						inactiveNotice: automation
							? scheduleInactiveNotice(automation)
							: undefined,
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
						canRotateSecret: true,
						onRotateSecret,
					}}
				/>
				<AutomationTargetField
					organizationId={organizationId}
					isCreate={isCreate}
					form={form}
					getFieldHelpers={getFieldHelpers}
					showProjectField={showProjectField}
					onWhenBusyChange={() => setWhenBusyChosen(true)}
				/>
			</fieldset>
			<AutomationRunsAsCard
				runsAsName={runsAsName}
				isWebhook={form.values.kind === "webhook"}
			/>
			{/* Next to the buttons, so a save error such as the per-owner
			    limit is in view where the user clicked. */}
			<div className="flex flex-col gap-4 border-0 border-t border-solid border-border pt-6">
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
				<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
					<Button
						type="button"
						variant="outline"
						disabled={isPending}
						onClick={onCancel}
					>
						Cancel
					</Button>
					{isCreate ? (
						<>
							<Button
								type="submit"
								variant="outline"
								disabled={isPending}
								onClick={() => setEnableOnCreate(false)}
							>
								<Spinner loading={isSubmitting && !enableOnCreate} />
								Save as disabled
							</Button>
							<Button
								type="submit"
								disabled={isPending}
								onClick={() => setEnableOnCreate(true)}
							>
								<Spinner loading={isSubmitting && enableOnCreate} />
								Save and enable
							</Button>
						</>
					) : (
						<Button type="submit" disabled={isPending}>
							<Spinner loading={isSubmitting} />
							Save changes
						</Button>
					)}
				</div>
			</div>
			<ConfirmDialog
				open={leavePrompt.isOpen}
				type="info"
				hideCancel={false}
				cancelText="Stay"
				title={prompt?.title ?? ""}
				description={prompt?.description}
				confirmText={prompt?.confirmText ?? "Leave"}
				onClose={leavePrompt.onCancel}
				onConfirm={leavePrompt.onConfirm}
				onCloseAutoFocus={leavePrompt.onCloseAutoFocus}
			/>
		</form>
	);
};

type AutomationRunsAsCardProps = {
	runsAsName: string;
	isWebhook: boolean;
};

/** The RFC's "Runs as" warning: whose permissions runs use, and what turning it off drops. */
export const AutomationRunsAsCard: React.FC<AutomationRunsAsCardProps> = ({
	runsAsName,
	isWebhook,
}) => (
	<div className="flex gap-3 rounded-lg border border-solid border-border bg-surface-secondary p-4">
		<TriangleAlertIcon
			aria-hidden
			className="mt-0.5 size-4 shrink-0 text-content-warning"
		/>
		<div className="flex flex-col gap-1 text-sm">
			<span className="font-medium text-content-primary">
				Runs as {runsAsName}
			</span>
			<span className="text-content-secondary">
				Runs use {runsAsName}'s Coder permissions and the chat's tools.{" "}
				{isWebhook &&
					"Anyone who has the secret can send event data to this agent, but the prompt stays the owner's. "}
				Turning the automation off blocks new runs and removes prompts it queued
				that have not started. Work that already started continues.
			</span>
		</div>
	</div>
);
