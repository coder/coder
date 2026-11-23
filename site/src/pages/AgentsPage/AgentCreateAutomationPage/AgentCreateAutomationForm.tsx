import cronstrue from "cronstrue";
import type { FormikProps } from "formik";
import { useEffect, useRef } from "react";
import type {
	Chat,
	ChatProject,
	CreateChatAutomationRequest,
	Organization,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Form,
	FormFields,
	FormFooter,
	FormSection,
} from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { Textarea } from "#/components/Textarea/Textarea";
import { formatReasoningEffort } from "#/modules/aiModels/helpers";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { getFormHelpers } from "#/utils/formUtils";
import { timeZones } from "#/utils/timeZones";
import { AutomationSearchField } from "./AutomationSearchField";

/** Unselected optional string fields use empty strings for controlled inputs. */
export type CreateAutomationFormValues =
	Required<CreateChatAutomationRequest> & {
		organization_id: string;
	};

type AgentCreateAutomationFormProps = {
	form: FormikProps<CreateAutomationFormValues>;
	organizations: readonly Organization[];
	modelOptions: readonly ModelSelectorOption[];
	projects: readonly ChatProject[];
	chats: readonly Chat[];
	loadingModels: boolean;
	loadingProjects: boolean;
	loadingChats: boolean;
	hasMoreChats: boolean;
	loadingMoreChats: boolean;
	onLoadMoreChats: () => void;
	isPending: boolean;
	submitError: unknown;
	onCancel: () => void;
};

/** Renders a Formik-controlled automation from organization-scoped resource options. */
export const AgentCreateAutomationForm: React.FC<
	AgentCreateAutomationFormProps
> = ({
	form,
	organizations,
	modelOptions,
	projects,
	chats,
	loadingModels,
	loadingProjects,
	loadingChats,
	hasMoreChats,
	loadingMoreChats,
	onLoadMoreChats,
	isPending,
	submitError,
	onCancel,
}: AgentCreateAutomationFormProps) => {
	const formRef = useRef<HTMLFormElement>(null);
	const errorRef = useRef<HTMLDivElement>(null);
	const focusedSubmit = useRef(0);
	useEffect(() => {
		if (
			form.isSubmitting ||
			form.isValidating ||
			focusedSubmit.current === form.submitCount
		) {
			return;
		}
		focusedSubmit.current = form.submitCount;
		const target =
			formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]') ??
			errorRef.current;
		target?.focus();
	}, [form.isSubmitting, form.isValidating, form.submitCount]);
	const getField = getFormHelpers<CreateAutomationFormValues>(
		form,
		submitError,
	);

	return (
		<Form
			ref={formRef}
			direction="vertical"
			onSubmit={form.handleSubmit}
			className="gap-10"
		>
			{Boolean(submitError) && (
				<div ref={errorRef} tabIndex={-1}>
					<ErrorAlert error={submitError} showDebugDetail={false} />
				</div>
			)}
			<FormSection
				title="Basic details"
				description="Name the automation and choose what it sends."
			>
				<FormFields>
					<FormField
						field={getField("organization_id")}
						label="Organization"
						required
						control={(control) => (
							<OrganizationAutocomplete
								{...control}
								required
								value={
									organizations.find(
										(org) => org.id === form.values.organization_id,
									) ?? null
								}
								options={organizations}
								onChange={(org) => {
									if (!org || org.id === form.values.organization_id) return;
									void form.setValues({
										...form.values,
										organization_id: org.id,
										new_chat_model_config_id: "",
										project_id: "",
										reasoning_effort: "",
										target_chat_id: "",
									});
								}}
								disabled={isPending}
							/>
						)}
					/>
					<FormField
						field={getField("name", {
							maxLength: 128,
							measureLength: (value) => Array.from(value.trim()).length,
						})}
						label="Name"
						placeholder="Weekly update"
						disabled={isPending}
					/>
					<FormField
						field={getField("prompt")}
						label="Prompt"
						control={(control) => (
							<Textarea
								{...form.getFieldProps("prompt")}
								{...control}
								rows={5}
								placeholder="What should the agent do?"
								disabled={isPending}
							/>
						)}
					/>
					<FormField
						field={getField("enabled")}
						label="Enabled"
						description="Create the automation in the enabled state."
						control={(control) => (
							<Switch
								{...control}
								checked={form.values.enabled}
								onCheckedChange={(enabled) => {
									void form.setFieldValue("enabled", enabled);
								}}
								disabled={isPending}
							/>
						)}
					/>
				</FormFields>
			</FormSection>

			<TriggerSection form={form} getField={getField} isPending={isPending} />

			<TargetSection
				form={form}
				getField={getField}
				modelOptions={modelOptions}
				projects={projects}
				chats={chats}
				loadingModels={loadingModels}
				loadingProjects={loadingProjects}
				loadingChats={loadingChats}
				hasMoreChats={hasMoreChats}
				loadingMoreChats={loadingMoreChats}
				onLoadMoreChats={onLoadMoreChats}
				isPending={isPending}
			/>
			<FormFooter>
				<Button
					type="button"
					variant="outline"
					onClick={onCancel}
					disabled={isPending}
				>
					Cancel
				</Button>
				<Button type="submit" disabled={isPending}>
					<Spinner loading={isPending} />
					Create automation
				</Button>
			</FormFooter>
		</Form>
	);
};

type SectionProps = Pick<
	AgentCreateAutomationFormProps,
	"form" | "isPending"
> & {
	getField: ReturnType<typeof getFormHelpers<CreateAutomationFormValues>>;
};

const TriggerSection: React.FC<SectionProps> = ({
	form,
	getField,
	isPending,
}) => {
	let cronHint: string;
	try {
		cronHint = cronstrue.toString(form.values.schedule_cron);
	} catch {
		cronHint = "Enter a five-field cron expression.";
	}

	return (
		<FormSection title="Trigger" description="Choose when the automation runs.">
			<FormFields>
				<FormField
					field={getField("kind")}
					label="Trigger type"
					control={(control) => (
						<Select
							value={form.values.kind}
							onValueChange={(kind: CreateAutomationFormValues["kind"]) => {
								void form.setValues({
									...form.values,
									kind,
									when_busy: kind === "schedule" ? "skip" : "queue",
									webhook_secret:
										kind === "webhook" ? form.values.webhook_secret : "",
								});
							}}
							disabled={isPending}
						>
							<SelectTrigger {...control}>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="schedule">Schedule</SelectItem>
								<SelectItem value="webhook">Webhook</SelectItem>
							</SelectContent>
						</Select>
					)}
				/>
				{form.values.kind === "schedule" ? (
					<>
						<FormField
							field={getField("schedule_cron", { helperText: cronHint })}
							label="Cron schedule"
							placeholder="0 9 * * 1-5"
							description="Five fields: minute, hour, day of month, month, day of week."
							disabled={isPending}
						/>
						<AutomationSearchField
							field={getField("schedule_time_zone")}
							label="Time zone"
							value={form.values.schedule_time_zone}
							onChange={(value) => {
								void form.setFieldValue("schedule_time_zone", value);
							}}
							options={timeZones.map((zone) => ({ value: zone, label: zone }))}
							placeholder="Select time zone"
							emptyMessage="No time zones found."
							disabled={isPending}
						/>
					</>
				) : (
					<>
						<FormField
							field={getField("webhook_auth")}
							label="Webhook authentication"
							control={(control) => (
								<Select
									value={form.values.webhook_auth}
									onValueChange={(
										webhook_auth: CreateAutomationFormValues["webhook_auth"],
									) => {
										void form.setValues({
											...form.values,
											webhook_auth,
											webhook_secret: "",
										});
									}}
									disabled={isPending}
								>
									<SelectTrigger {...control}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="bearer">Bearer token</SelectItem>
										<SelectItem value="github">GitHub signature</SelectItem>
										<SelectItem value="standard_webhooks">
											Standard Webhooks
										</SelectItem>
									</SelectContent>
								</Select>
							)}
						/>
						{form.values.webhook_auth === "standard_webhooks" ? (
							<FormField
								field={getField("webhook_secret", {
									helperText: "Enter the sender's whsec_ secret.",
								})}
								label="Webhook secret"
								type="password"
								autoComplete="off"
								ignorePasswordManagers
								disabled={isPending}
							/>
						) : (
							<p className="text-sm text-content-secondary">
								Coder generates a webhook secret when you create this
								automation. Copy it from the confirmation; it is returned only
								once.
							</p>
						)}
					</>
				)}
			</FormFields>
		</FormSection>
	);
};

type TargetSectionProps = SectionProps &
	Pick<
		AgentCreateAutomationFormProps,
		| "modelOptions"
		| "projects"
		| "chats"
		| "loadingModels"
		| "loadingProjects"
		| "loadingChats"
		| "hasMoreChats"
		| "loadingMoreChats"
		| "onLoadMoreChats"
	>;

const TargetSection: React.FC<TargetSectionProps> = ({
	form,
	getField,
	modelOptions,
	projects,
	chats,
	loadingModels,
	loadingProjects,
	loadingChats,
	hasMoreChats,
	loadingMoreChats,
	onLoadMoreChats,
	isPending,
}) => {
	const selectedModel = modelOptions.find(
		(option) => option.id === form.values.new_chat_model_config_id,
	);
	const reasoningEfforts = selectedModel?.reasoningEfforts ?? [];

	return (
		<FormSection
			title="Target"
			description="Choose where each run sends the prompt."
		>
			<FormFields>
				<FormField
					field={getField("target_mode")}
					label="Target mode"
					control={(control) => (
						<Select
							value={form.values.target_mode}
							onValueChange={(
								value: CreateAutomationFormValues["target_mode"],
							) => {
								void form.setFieldValue("target_mode", value);
							}}
							disabled={isPending}
						>
							<SelectTrigger {...control}>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="new_chat">Start a new chat</SelectItem>
								<SelectItem value="existing_chat">
									Use an existing chat
								</SelectItem>
							</SelectContent>
						</Select>
					)}
				/>
				{form.values.target_mode === "new_chat" ? (
					<>
						<AutomationSearchField
							field={getField("new_chat_model_config_id")}
							label="Model"
							value={form.values.new_chat_model_config_id}
							onChange={(value) => {
								void form.setValues({
									...form.values,
									new_chat_model_config_id: value,
									reasoning_effort: "",
								});
							}}
							options={modelOptions.map((model) => ({
								value: model.id,
								label: `${model.displayName} (${model.providerLabel || model.provider})`,
							}))}
							placeholder="Select model"
							emptyMessage="No models available."
							disabled={isPending}
							loading={loadingModels}
						/>
						<AutomationSearchField
							field={getField("project_id")}
							label="Project"
							value={form.values.project_id}
							onChange={(value) => {
								void form.setFieldValue("project_id", value);
							}}
							options={projects.map((project) => ({
								value: project.id,
								label: project.name,
							}))}
							placeholder="Select project"
							noneLabel="None"
							emptyMessage="No projects found."
							disabled={isPending}
							loading={loadingProjects}
						/>
						{reasoningEfforts.length > 0 && (
							<FormField
								field={getField("reasoning_effort")}
								label="Reasoning effort"
								control={(control) => (
									<Select
										value={form.values.reasoning_effort || "model_default"}
										onValueChange={(value) => {
											void form.setFieldValue(
												"reasoning_effort",
												value === "model_default" ? "" : value,
											);
										}}
										disabled={isPending}
									>
										<SelectTrigger {...control}>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="model_default">
												Model default
											</SelectItem>
											{reasoningEfforts.map((effort) => (
												<SelectItem key={effort} value={effort}>
													{formatReasoningEffort(effort)}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								)}
							/>
						)}
					</>
				) : (
					<>
						<AutomationSearchField
							field={getField("target_chat_id")}
							label="Existing chat"
							value={form.values.target_chat_id}
							onChange={(value) => {
								void form.setFieldValue("target_chat_id", value);
							}}
							options={chats.map((chat) => ({
								value: chat.id,
								label: chat.title || "Untitled chat",
							}))}
							placeholder="Select chat"
							emptyMessage="No active chats found."
							disabled={isPending}
							loading={loadingChats}
							hasMore={hasMoreChats}
							loadingMore={loadingMoreChats}
							onLoadMore={onLoadMoreChats}
						/>
						<p className="text-sm text-content-secondary">
							Choose one of your active root chats. It keeps its model and
							project.
						</p>
						<FormField
							field={getField("when_busy")}
							label="When the chat is busy"
							control={(control) => (
								<Select
									value={form.values.when_busy}
									onValueChange={(
										value: CreateAutomationFormValues["when_busy"],
									) => {
										void form.setFieldValue("when_busy", value);
									}}
									disabled={isPending}
								>
									<SelectTrigger {...control}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="queue">Queue</SelectItem>
										<SelectItem value="skip">Skip</SelectItem>
									</SelectContent>
								</Select>
							)}
						/>
					</>
				)}
			</FormFields>
		</FormSection>
	);
};
