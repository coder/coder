import { useId } from "react";
import { useQuery } from "react-query";
import { getErrorMessage, isApiValidationError } from "#/api/errors";
import { templateVersionPresets } from "#/api/queries/templates";
import type { CreateChatProjectRequest, Template } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Label } from "#/components/Label/Label";
import { MultiSelectCombobox } from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { SettingSwitch } from "../../SettingSwitch";
import { ChatProjectDefaultParameters } from "./ChatProjectDefaultParameters";

/** The default template is always one of `templateIds`. */
export type ChatProjectTemplatesValues = {
	/** Whether chats can only use the selected templates. */
	limitTemplates: boolean;
	templateIds: readonly string[];
	/** Empty when the project has no default template. */
	defaultTemplateId: string;
	/**
	 * Picks which of `defaultPresetId` or `defaultParameters` is saved; the
	 * other is ignored.
	 */
	defaultsKind: "preset" | "parameters";
	/** Empty when the default template has no default preset. */
	defaultPresetId: string;
	/**
	 * Values by parameter name. A missing name uses the template default; an
	 * empty string is an explicit empty value.
	 */
	defaultParameters: Readonly<Record<string, string>>;
};

/** The value of a select's None option, which stands for an empty ID. */
const noSelection = "none";

/** The create request, the saved state, and a draft all take this shape. */
export type ChatProjectTemplatesSettings = Required<
	Pick<CreateChatProjectRequest, "limit_templates" | "template_ids">
> &
	Pick<CreateChatProjectRequest, "default_template">;

/** Returns the form state for saved templates, or an empty form when none. */
export const chatProjectTemplatesValues = (
	templates: ChatProjectTemplatesSettings | undefined,
): ChatProjectTemplatesValues => ({
	limitTemplates: templates?.limit_templates ?? false,
	templateIds: templates?.template_ids ?? [],
	defaultTemplateId: templates?.default_template?.template_id ?? "",
	defaultsKind: templates?.default_template?.parameters
		? "parameters"
		: "preset",
	defaultPresetId: templates?.default_template?.preset_id ?? "",
	defaultParameters: templates?.default_template?.parameters ?? {},
});

/**
 * Builds the request from the form values. Templates that are no longer
 * eligible (deleted, deprecated, or no longer allowed for agents) are
 * dropped, since the server would reject them.
 */
export const chatProjectTemplatesRequest = (
	values: ChatProjectTemplatesValues,
	eligible: readonly Template[],
): ChatProjectTemplatesSettings => {
	const request = {
		limit_templates: values.limitTemplates,
		template_ids: values.templateIds.filter((id) =>
			eligible.some((template) => template.id === id),
		),
	};
	if (!request.template_ids.includes(values.defaultTemplateId)) {
		return request;
	}
	if (values.defaultsKind === "preset") {
		return {
			...request,
			default_template: {
				template_id: values.defaultTemplateId,
				preset_id: values.defaultPresetId || undefined,
			},
		};
	}
	const parameters = values.defaultParameters;
	return {
		...request,
		default_template: {
			template_id: values.defaultTemplateId,
			parameters: Object.keys(parameters).length > 0 ? parameters : undefined,
		},
	};
};

/**
 * Returns the server's rejection of the templates in a save error. Forms
 * have no field named after these validations, so the templates fields show
 * it through `saveError`.
 */
export const templatesValidationDetail = (error: unknown) =>
	isApiValidationError(error)
		? error.response.data.validations?.find(
				(validation) =>
					validation.field === "template_ids" ||
					validation.field.startsWith("default_template."),
			)?.detail
		: undefined;

const templateLabel = (template: Template) =>
	template.display_name || template.name;

/** Explains why the templates list has nothing left to pick. */
const noTemplatesMessage = (optionCount: number, selectedCount: number) => {
	if (optionCount === 0) {
		return "No templates in this organization allow agents.";
	}
	if (optionCount === selectedCount) {
		return "Every template is selected.";
	}
	return "No matching templates.";
};

type ChatProjectTemplatesFieldsProps = {
	/** Templates in the project's organization that allow agents. */
	readonly templates: readonly Template[] | undefined;
	readonly templatesError: unknown;
	/** The server's rejection of the saved templates, if any. */
	readonly saveError: string | undefined;
	readonly value: ChatProjectTemplatesValues;
	readonly onChange: (value: ChatProjectTemplatesValues) => void;
	readonly disabled: boolean;
};

/**
 * Only selected templates can be the default template. Deselecting the
 * default template, or picking another one, clears its preset and parameter
 * values.
 */
export const ChatProjectTemplatesFields: React.FC<
	ChatProjectTemplatesFieldsProps
> = ({ templates, templatesError, saveError, value, onChange, disabled }) => {
	const defaultTemplateSelectId = useId();
	const limitsToNothing =
		value.limitTemplates && value.templateIds.length === 0;
	const options = (templates ?? []).map((template) => ({
		value: template.id,
		label: templateLabel(template),
	}));
	const selectedTemplates = (templates ?? []).filter((template) =>
		value.templateIds.includes(template.id),
	);
	const defaultTemplate = selectedTemplates.find(
		(template) => template.id === value.defaultTemplateId,
	);

	const setTemplateIds = (templateIds: readonly string[]) => {
		if (templateIds.includes(value.defaultTemplateId)) {
			onChange({ ...value, templateIds });
			return;
		}
		onChange({
			...value,
			templateIds,
			defaultTemplateId: "",
			defaultPresetId: "",
			defaultParameters: {},
		});
	};

	return (
		<>
			<div className="flex flex-col gap-2">
				<div className="flex items-center justify-between">
					{/* cmdk overrides the input's id and aria-labelledby, so the
					    combobox is named through commandProps instead. */}
					<Label>Templates</Label>
					<Button
						type="button"
						variant="subtle"
						size="sm"
						disabled={disabled || options.length === 0}
						onClick={() => setTemplateIds(options.map((o) => o.value))}
					>
						Select all
					</Button>
				</div>
				{templates && (
					<MultiSelectCombobox
						commandProps={{ label: "Templates" }}
						options={options}
						value={options.filter((o) => value.templateIds.includes(o.value))}
						onChange={(selected) =>
							setTemplateIds(selected.map((o) => o.value))
						}
						placeholder="Select templates"
						emptyIndicator={
							<p className="m-0 text-center text-sm text-content-secondary">
								{noTemplatesMessage(options.length, value.templateIds.length)}
							</p>
						}
						disabled={disabled}
						hidePlaceholderWhenSelected
					/>
				)}
				{!templates && !templatesError && <Skeleton className="h-10 w-full" />}
				{/* Shown with cached templates too, since a failed refetch means
				    the list may be out of date. */}
				{Boolean(templatesError) && (
					<p role="alert" className="m-0 text-sm text-content-destructive">
						{getErrorMessage(templatesError, "Failed to load templates.")}
					</p>
				)}
				<p className="m-0 text-xs text-content-secondary">
					Agents see these templates first when they choose a template for a
					workspace.
				</p>
			</div>
			<SettingSwitch
				label="Limit chats to these templates"
				description={
					<>
						<p className="m-0">
							Chats in this project can only create or attach workspaces that
							use these templates. When this is off, chats can use any template
							their owner can use.
						</p>
						{limitsToNothing && (
							<p className="m-0 text-content-warning">
								With no templates selected, chats in this project can't create
								or attach workspaces.
							</p>
						)}
					</>
				}
				checked={value.limitTemplates}
				disabled={disabled}
				onCheckedChange={(limitTemplates) =>
					onChange({ ...value, limitTemplates })
				}
			/>
			{selectedTemplates.length > 0 && (
				<div className="flex flex-col gap-2">
					<Label htmlFor={defaultTemplateSelectId}>Default template</Label>
					<Select
						value={value.defaultTemplateId || noSelection}
						onValueChange={(id) =>
							onChange({
								...value,
								defaultTemplateId: id === noSelection ? "" : id,
								defaultPresetId: "",
								defaultParameters: {},
							})
						}
						disabled={disabled}
					>
						<SelectTrigger id={defaultTemplateSelectId}>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value={noSelection}>None</SelectItem>
							{selectedTemplates.map((template) => (
								<SelectItem key={template.id} value={template.id}>
									{templateLabel(template)}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
			)}
			{defaultTemplate && (
				<ChatProjectDefaultsFields
					template={defaultTemplate}
					value={value}
					onChange={onChange}
					disabled={disabled}
				/>
			)}
			{saveError && (
				<p role="alert" className="m-0 text-sm text-content-destructive">
					{saveError}
				</p>
			)}
		</>
	);
};

type ChatProjectDefaultsFieldsProps = {
	readonly template: Template;
	readonly value: ChatProjectTemplatesValues;
	readonly onChange: (value: ChatProjectTemplatesValues) => void;
	readonly disabled: boolean;
};

const ChatProjectDefaultsFields: React.FC<ChatProjectDefaultsFieldsProps> = ({
	template,
	value,
	onChange,
	disabled,
}) => {
	const presetKindId = useId();
	const parametersKindId = useId();

	return (
		<div className="flex flex-col gap-3">
			<RadioGroup
				aria-label="Default values"
				className="flex gap-4"
				value={value.defaultsKind}
				onValueChange={(kind) =>
					onChange({
						...value,
						defaultsKind: kind === "parameters" ? "parameters" : "preset",
						defaultPresetId: "",
						defaultParameters: {},
					})
				}
				disabled={disabled}
			>
				<div className="flex items-center gap-2">
					<RadioGroupItem id={presetKindId} value="preset" />
					<Label htmlFor={presetKindId} className="font-normal">
						Preset
					</Label>
				</div>
				<div className="flex items-center gap-2">
					<RadioGroupItem id={parametersKindId} value="parameters" />
					<Label htmlFor={parametersKindId} className="font-normal">
						Parameter values
					</Label>
				</div>
			</RadioGroup>
			{value.defaultsKind === "preset" && (
				<ChatProjectDefaultPreset
					templateVersionId={template.active_version_id}
					value={value.defaultPresetId}
					onChange={(defaultPresetId) =>
						onChange({ ...value, defaultPresetId })
					}
					disabled={disabled}
				/>
			)}
			{value.defaultsKind === "parameters" && (
				// Keyed by version, so another template or a new active version
				// never shows the previous one's parameters.
				<ChatProjectDefaultParameters
					key={template.active_version_id}
					templateVersionId={template.active_version_id}
					value={value.defaultParameters}
					onChange={(defaultParameters) =>
						onChange({ ...value, defaultParameters })
					}
					disabled={disabled}
				/>
			)}
		</div>
	);
};

type ChatProjectDefaultPresetProps = {
	readonly templateVersionId: string;
	/** Empty when no preset is selected. */
	readonly value: string;
	readonly onChange: (presetId: string) => void;
	readonly disabled: boolean;
};

const ChatProjectDefaultPreset: React.FC<ChatProjectDefaultPresetProps> = ({
	templateVersionId,
	value,
	onChange,
	disabled,
}) => {
	const selectId = useId();
	const presetsQuery = useQuery(templateVersionPresets(templateVersionId));

	if (presetsQuery.isPending) {
		return <Skeleton className="h-10 w-full" />;
	}
	// The server returns null rather than [] for a version without presets.
	const presets = presetsQuery.data ?? [];

	return (
		<>
			{presetsQuery.data !== undefined && presets.length === 0 && (
				<p className="m-0 text-sm text-content-secondary">
					This template has no presets.
				</p>
			)}
			{presets.length > 0 && (
				<div className="flex flex-col gap-2">
					<Label htmlFor={selectId}>Default preset</Label>
					<Select
						value={value || noSelection}
						onValueChange={(id) => onChange(id === noSelection ? "" : id)}
						disabled={disabled}
					>
						<SelectTrigger id={selectId}>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value={noSelection}>None</SelectItem>
							{presets.map((preset) => (
								<SelectItem key={preset.ID} value={preset.ID}>
									{preset.Name}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
			)}
			{/* Shown with cached presets too, since a failed refetch means they
			    may be out of date. */}
			{presetsQuery.error && (
				<p role="alert" className="m-0 text-sm text-content-destructive">
					{getErrorMessage(presetsQuery.error, "Failed to load presets.")}
				</p>
			)}
		</>
	);
};
