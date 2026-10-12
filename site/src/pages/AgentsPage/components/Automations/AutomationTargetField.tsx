import type { FormikContextType } from "formik";
import { useId } from "react";
import { useQuery } from "react-query";
import { chat, chatModels } from "#/api/queries/chats";
import {
	ChatModelReasoningEffortHigh,
	ChatModelReasoningEffortLow,
	ChatModelReasoningEffortMax,
	ChatModelReasoningEffortMedium,
	ChatModelReasoningEffortMinimal,
	ChatModelReasoningEffortNone,
} from "#/api/typesGenerated";
import { FormField } from "#/components/FormField/FormField";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
import { formatReasoningEffort } from "#/modules/aiModels/helpers";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import type { FormHelpers } from "#/utils/formUtils";
import {
	getModelSelectorPlaceholder,
	hasUserFixableProviders,
	resolveModelSelector,
} from "../../utils/modelOptions";
import { getModelSelectorHelp } from "../ModelSelectorHelp";
import { AutomationChatPicker } from "./AutomationChatPicker";
import type { AutomationFormValues } from "./AutomationEditorForm";
import { AutomationProjectField } from "./AutomationProjectField";
import { FixedAtCreationBadge } from "./FixedAtCreationBadge";
import { RadioCards } from "./RadioCards";

// Radix Select rejects an empty item value, so Model default has its own.
const MODEL_DEFAULT_EFFORT = "model-default";

// The server accepts any of these for a new_chat automation, whatever the
// model lists, so a model that lists none still offers all of them.
const ALL_REASONING_EFFORTS = [
	ChatModelReasoningEffortNone,
	ChatModelReasoningEffortMinimal,
	ChatModelReasoningEffortLow,
	ChatModelReasoningEffortMedium,
	ChatModelReasoningEffortHigh,
	ChatModelReasoningEffortMax,
];

type AutomationTargetFieldProps = {
	organizationId: string;
	isCreate: boolean;
	form: FormikContextType<AutomationFormValues>;
	getFieldHelpers: (name: keyof AutomationFormValues) => FormHelpers;
	/** Shows the Project field for new chat targets. */
	showProjectField: boolean;
	onWhenBusyChange: () => void;
};

/** Where each run sends its prompt: an existing chat or a new chat. */
export const AutomationTargetField: React.FC<AutomationTargetFieldProps> = ({
	organizationId,
	isCreate,
	form,
	getFieldHelpers,
	showProjectField,
	onWhenBusyChange,
}) => {
	const headingId = useId();
	const fixedId = useId();
	const modelErrorId = useId();
	const effortId = useId();
	const effortErrorId = useId();
	const modelsQuery = useQuery(chatModels(organizationId));
	const {
		options: modelOptions,
		isModelCatalogLoading,
		modelCatalog,
		hasConfiguredModels,
	} = resolveModelSelector(organizationId, modelsQuery);
	const modelSelectorHelp = getModelSelectorHelp({
		isModelCatalogLoading,
		hasModelOptions: modelOptions.length > 0,
		hasConfiguredModels,
		hasUserFixableModelProviders: hasUserFixableProviders(modelCatalog),
	});
	const modelField = getFieldHelpers("new_chat_model_config_id");
	const effortField = getFieldHelpers("reasoning_effort");
	const selectedModel = modelOptions.find(
		(option) => option.id === form.values.new_chat_model_config_id,
	);
	// A saved model the catalog no longer lists, for example a disabled one.
	const isSavedModelUnavailable =
		!isModelCatalogLoading &&
		!modelsQuery.isError &&
		form.values.new_chat_model_config_id !== "" &&
		!selectedModel;
	const reasoningEffort = form.values.reasoning_effort;
	const listedEfforts = selectedModel?.reasoningEfforts ?? [];
	const efforts =
		selectedModel && listedEfforts.length === 0
			? ALL_REASONING_EFFORTS
			: listedEfforts;
	// Keeps a stored effort visible after the model stops listing it.
	const effortOptions =
		reasoningEffort && !efforts.includes(reasoningEffort)
			? [...efforts, reasoningEffort]
			: efforts;

	const isExistingChat = form.values.target_mode === "existing_chat";
	const runsInOptions = [
		{
			value: "new_chat",
			label: "New chat each run",
			description: "Each run starts a new chat with the model below.",
		},
		{
			value: "existing_chat",
			label: "Existing chat",
			description: "Each run sends the prompt to the chosen chat.",
			// The target chat stays editable after creation, so its picker
			// lives in the card, apart from the fixed choice of mode.
			children: isExistingChat && (
				<div className="flex flex-col gap-3">
					<FormField
						field={getFieldHelpers("target_chat_id")}
						label="Chat"
						control={(props) => (
							<AutomationChatPicker
								{...props}
								organizationId={organizationId}
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
								onWhenBusyChange();
								form.setFieldValue("when_busy", value);
							}
						}}
					>
						<SelectItem value="skip">Skip the run</SelectItem>
						<SelectItem value="queue">Queue the prompt</SelectItem>
					</SelectField>
				</div>
			),
		},
	] as const;

	return (
		<>
			<section className="flex flex-col gap-3">
				<div className="flex flex-wrap items-center justify-between gap-2">
					<h3
						id={headingId}
						className="m-0 text-sm font-medium text-content-primary"
					>
						Runs in
					</h3>
					{!isCreate && <FixedAtCreationBadge id={fixedId} />}
				</div>
				<RadioCards
					aria-labelledby={headingId}
					aria-describedby={isCreate ? undefined : fixedId}
					value={form.values.target_mode}
					disabled={!isCreate}
					options={runsInOptions}
					onValueChange={(value) => form.setFieldValue("target_mode", value)}
				/>
			</section>
			<section className="flex flex-col gap-3">
				<h3 className="m-0 text-sm font-medium text-content-primary">
					Environment
				</h3>
				{isExistingChat ? (
					<EffectiveModel
						chatId={form.values.target_chat_id}
						modelOptions={modelOptions}
					/>
				) : (
					<div className="flex flex-col gap-2">
						<div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
							<div className="flex flex-col gap-2">
								<span className="flex items-center gap-1 text-sm font-medium text-content-primary">
									Model
									<InfoTooltip size="small" ariaLabel="About Model">
										Model for each new chat.
									</InfoTooltip>
								</span>
								<div>
									<ModelSelector
										// Matches the Select next to it instead of the composer pill.
										className="h-10 w-full justify-between rounded-md border border-solid border-border bg-transparent px-3 text-sm shadow-xs hover:bg-surface-secondary"
										// As wide as the trigger. The list keeps its own fixed
										// height so Radix can flip it above the trigger.
										contentClassName="w-[max(18rem,var(--radix-popover-trigger-width))]"
										triggerAriaLabel="Model"
										triggerAriaInvalid={modelField.error}
										triggerAriaDescribedBy={
											modelField.error ? modelErrorId : undefined
										}
										placeholder={
											isSavedModelUnavailable
												? "Unavailable model"
												: getModelSelectorPlaceholder(
														modelOptions,
														isModelCatalogLoading,
														hasConfiguredModels,
														modelCatalog,
													)
										}
										options={modelOptions}
										value={form.values.new_chat_model_config_id}
										onValueChange={(modelId) => {
											if (modelId !== form.values.new_chat_model_config_id) {
												// One update: Formik validates each setFieldValue against the
												// values of the last render, so a second call would bring
												// back "Choose a model.".
												void form.setValues({
													...form.values,
													new_chat_model_config_id: modelId,
													reasoning_effort: "",
												});
											}
										}}
									/>
									<div aria-live="polite">
										{modelField.error && (
											<p
												id={modelErrorId}
												className="m-0 mt-2 text-xs text-content-destructive"
											>
												{modelField.helperText}
											</p>
										)}
									</div>
								</div>
							</div>
							<div className="flex flex-col gap-2">
								<span className="flex items-center gap-1">
									<Label htmlFor={effortId}>Reasoning effort</Label>
									<InfoTooltip size="small" ariaLabel="About Reasoning effort">
										How much the model reasons before answering. Model default
										uses the model's own setting.
									</InfoTooltip>
								</span>
								<Select
									value={reasoningEffort || MODEL_DEFAULT_EFFORT}
									disabled={effortOptions.length === 0}
									onValueChange={(value) =>
										form.setFieldValue(
											"reasoning_effort",
											value === MODEL_DEFAULT_EFFORT ? "" : value,
										)
									}
								>
									<SelectTrigger
										id={effortId}
										aria-invalid={effortField.error}
										aria-describedby={
											effortField.error ? effortErrorId : undefined
										}
									>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value={MODEL_DEFAULT_EFFORT}>
											Model default
										</SelectItem>
										{effortOptions.map((effort) => (
											<SelectItem key={effort} value={effort}>
												{formatReasoningEffort(effort)}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
								{effortField.error && (
									<span
										id={effortErrorId}
										className="text-xs text-content-destructive"
									>
										{effortField.helperText}
									</span>
								)}
							</div>
							{showProjectField && (
								<AutomationProjectField
									organizationId={organizationId}
									field={getFieldHelpers("project_id")}
									onValueChange={(value) =>
										form.setFieldValue("project_id", value)
									}
								/>
							)}
						</div>
						{isSavedModelUnavailable && (
							<span className="text-xs text-content-warning">
								The saved model is disabled or was removed, so runs are paused.
								Choose another model and save.
							</span>
						)}
						{selectedModel && listedEfforts.length === 0 && (
							<span className="text-xs text-content-secondary">
								This model does not list its reasoning efforts. The provider can
								ignore or reject a value you choose.
							</span>
						)}
						{modelSelectorHelp && (
							<span className="text-xs text-content-secondary">
								{modelSelectorHelp}
							</span>
						)}
						{modelsQuery.isError && (
							<span className="text-xs text-content-destructive">
								Could not load models.
							</span>
						)}
					</div>
				)}
			</section>
		</>
	);
};

type EffectiveModelProps = {
	chatId: string;
	modelOptions: readonly ModelSelectorOption[];
};

/** The model an existing-chat run uses: the chat's own, which the chat sets. */
const EffectiveModel: React.FC<EffectiveModelProps> = ({
	chatId,
	modelOptions,
}) => {
	const id = useId();
	const chatQuery = useQuery({ ...chat(chatId), enabled: Boolean(chatId) });
	const modelId = chatQuery.data?.last_model_config_id;
	let value = "Choose a chat";
	if (chatId && chatQuery.isLoading) {
		value = "Loading";
	} else if (chatQuery.isError) {
		value = "Could not load the chat";
	} else if (modelId) {
		value =
			modelOptions.find((option) => option.id === modelId)?.displayName ??
			"Unavailable model";
	}
	return (
		<div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<div className="flex min-w-0 flex-col gap-2">
				<span className="flex items-center gap-1">
					<Label htmlFor={id}>Model (effective)</Label>
					<InfoTooltip size="small" ariaLabel="About Model (effective)">
						Runs use the chat's model. Change it in the chat.
					</InfoTooltip>
				</span>
				<Input id={id} value={value} readOnly />
			</div>
		</div>
	);
};
