import type { FormikContextType } from "formik";
import { useId } from "react";
import { useQuery } from "react-query";
import { chatModels } from "#/api/queries/chats";
import { FormField } from "#/components/FormField/FormField";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Label } from "#/components/Label/Label";
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
import { formatReasoningEffort } from "#/modules/aiModels/helpers";
import { ModelSelector } from "#/modules/aiModels/ModelSelector";
import type { FormHelpers } from "#/utils/formUtils";
import {
	getModelSelectorPlaceholder,
	hasUserFixableProviders,
	resolveModelSelector,
} from "../../utils/modelOptions";
import { getModelSelectorHelp } from "../ModelSelectorHelp";
import { AutomationChatPicker } from "./AutomationChatPicker";
import type { AutomationFormValues } from "./AutomationEditorDialog";
import { FixedAtCreationBadge } from "./FixedAtCreationBadge";
import { RadioOption } from "./RadioOption";

// Radix Select rejects an empty item value, so Model default has its own.
const MODEL_DEFAULT_EFFORT = "model-default";

type AutomationTargetFieldProps = {
	organizationId: string;
	isCreate: boolean;
	form: FormikContextType<AutomationFormValues>;
	getFieldHelpers: (name: keyof AutomationFormValues) => FormHelpers;
	onWhenBusyChange: () => void;
};

/** Where each run sends its prompt: an existing chat or a new chat. */
export const AutomationTargetField: React.FC<AutomationTargetFieldProps> = ({
	organizationId,
	isCreate,
	form,
	getFieldHelpers,
	onWhenBusyChange,
}) => {
	const headingId = useId();
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
	const isExistingChat = form.values.target_mode === "existing_chat";
	const selectedModel = modelOptions.find(
		(option) => option.id === form.values.new_chat_model_config_id,
	);
	const reasoningEffort = form.values.reasoning_effort;
	const efforts = selectedModel?.reasoningEfforts ?? [];
	// Keeps a stored effort visible after the model stops listing it.
	const effortOptions =
		reasoningEffort && !efforts.includes(reasoningEffort)
			? [...efforts, reasoningEffort]
			: efforts;

	return (
		<section className="flex flex-col gap-4">
			<div className="flex items-center justify-between gap-2">
				<h3
					id={headingId}
					className="m-0 text-sm font-medium text-content-primary"
				>
					Runs in
				</h3>
				{!isCreate && <FixedAtCreationBadge />}
			</div>
			<RadioGroup
				aria-labelledby={headingId}
				value={form.values.target_mode}
				disabled={!isCreate}
				onValueChange={(value) => {
					if (value === "existing_chat" || value === "new_chat") {
						form.setFieldValue("target_mode", value);
					}
				}}
			>
				<RadioOption
					value="existing_chat"
					label="Existing chat"
					tooltip="Each run sends the prompt to the chosen chat."
				/>
				<RadioOption
					value="new_chat"
					label="New chat each run"
					tooltip="Each run starts a new chat with the model below."
				/>
			</RadioGroup>
			{isExistingChat ? (
				<div className="grid grid-cols-2 gap-4">
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
			) : (
				<div className="flex flex-col gap-2">
					<div className="grid grid-cols-2 gap-4">
						<div className="flex flex-col gap-2">
							<span className="flex items-center gap-1 text-sm font-medium text-content-primary">
								Model
								<InfoTooltip size="small" ariaLabel="About Model">
									Model for each new chat.
								</InfoTooltip>
							</span>
							<div>
								<ModelSelector
									className="w-full"
									triggerAriaLabel="Model"
									triggerAriaInvalid={modelField.error}
									triggerAriaDescribedBy={
										modelField.error ? modelErrorId : undefined
									}
									placeholder={getModelSelectorPlaceholder(
										modelOptions,
										isModelCatalogLoading,
										hasConfiguredModels,
										modelCatalog,
									)}
									options={modelOptions}
									value={form.values.new_chat_model_config_id}
									onValueChange={(modelId) => {
										if (modelId !== form.values.new_chat_model_config_id) {
											form.setFieldValue("new_chat_model_config_id", modelId);
											form.setFieldValue("reasoning_effort", "");
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
					</div>
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
	);
};
