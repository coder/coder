import { useId } from "react";
import { useQuery } from "react-query";
import { getErrorMessage, isApiValidationError } from "#/api/errors";
import { templateVersionPresets } from "#/api/queries/templates";
import type { CreateChatProjectRequest, Template } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Label } from "#/components/Label/Label";
import { MultiSelectCombobox } from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { SettingSwitch } from "../../SettingSwitch";

/** Form state for a project's templates and the defaults its chats use. */
export type ChatProjectTemplatesValues = {
	/** Whether chats can only use the selected templates. */
	limitTemplates: boolean;
	templateIds: readonly string[];
	/** Empty when the project has no default template. */
	defaultTemplateId: string;
	defaultPresetId: string;
};

const noDefaultTemplate = "none";

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
	defaultPresetId: templates?.default_template?.preset_id ?? "",
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
	return {
		...request,
		default_template: {
			template_id: values.defaultTemplateId,
			preset_id: values.defaultPresetId || undefined,
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
 * Fields that pick a project's templates, whether its chats are limited to
 * them, an optional default template, and that template's default preset.
 */
export const ChatProjectTemplatesFields: React.FC<
	ChatProjectTemplatesFieldsProps
> = ({ templates, templatesError, saveError, value, onChange, disabled }) => {
	const defaultTemplateId = useId();
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
					<Label htmlFor={defaultTemplateId}>Default template</Label>
					<Select
						value={value.defaultTemplateId || noDefaultTemplate}
						onValueChange={(id) =>
							onChange({
								...value,
								defaultTemplateId: id === noDefaultTemplate ? "" : id,
								defaultPresetId: "",
							})
						}
						disabled={disabled}
					>
						<SelectTrigger id={defaultTemplateId}>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value={noDefaultTemplate}>None</SelectItem>
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
				<ChatProjectDefaultPreset
					templateVersionId={defaultTemplate.active_version_id}
					value={value.defaultPresetId}
					onChange={(defaultPresetId) =>
						onChange({ ...value, defaultPresetId })
					}
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
					<Select value={value} onValueChange={onChange} disabled={disabled}>
						<SelectTrigger id={selectId}>
							<SelectValue placeholder="Select a preset" />
						</SelectTrigger>
						<SelectContent>
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
