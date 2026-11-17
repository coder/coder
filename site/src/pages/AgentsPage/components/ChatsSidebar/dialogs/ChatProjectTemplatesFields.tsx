import { useId } from "react";
import { useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
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

/** Form state for a project's templates and the defaults its chats use. */
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
	defaultPresetId: string;
	/**
	 * Values by parameter name. A missing name uses the template default; an
	 * empty string is an explicit empty value.
	 */
	defaultParameters: Readonly<Record<string, string>>;
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

const templateLabel = (template: Template) =>
	template.display_name || template.name;

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
 * them, an optional default template, and that template's default preset or
 * parameter values.
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
				{templates ? (
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
								{options.length === 0
									? "No templates in this organization allow agents."
									: options.length === value.templateIds.length
										? "Every template is selected."
										: "No matching templates."}
							</p>
						}
						disabled={disabled}
						hidePlaceholderWhenSelected
					/>
				) : (
					!templatesError && <Skeleton className="h-10 w-full" />
				)}
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
								defaultParameters: {},
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
	const presetId = useId();
	const presetsQuery = useQuery({
		...templateVersionPresets(template.active_version_id),
		enabled: value.defaultsKind === "preset",
	});
	// The server returns null rather than [] for a version without presets.
	const presets =
		presetsQuery.data === undefined ? undefined : (presetsQuery.data ?? []);

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
			{value.defaultsKind === "preset" ? (
				presets ? (
					presets.length === 0 ? (
						<p className="m-0 text-sm text-content-secondary">
							This template has no presets.
						</p>
					) : (
						<div className="flex flex-col gap-2">
							<Label htmlFor={presetId}>Default preset</Label>
							<Select
								value={value.defaultPresetId}
								onValueChange={(id) =>
									onChange({ ...value, defaultPresetId: id })
								}
								disabled={disabled}
							>
								<SelectTrigger id={presetId}>
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
					)
				) : presetsQuery.error ? (
					<p role="alert" className="m-0 text-sm text-content-destructive">
						{getErrorMessage(presetsQuery.error, "Failed to load presets.")}
					</p>
				) : (
					<Skeleton className="h-10 w-full" />
				)
			) : (
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
