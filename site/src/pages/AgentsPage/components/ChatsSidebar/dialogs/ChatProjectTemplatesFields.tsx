import { getErrorMessage, isApiValidationError } from "#/api/errors";
import type { CreateChatProjectRequest, Template } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Label } from "#/components/Label/Label";
import { MultiSelectCombobox } from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { SettingSwitch } from "../../SettingSwitch";

/** Form state for a project's templates. */
export type ChatProjectTemplatesValues = {
	/** Whether chats can only use the selected templates. */
	limitTemplates: boolean;
	templateIds: readonly string[];
};

/** The create request, the saved state, and a draft all take this shape. */
export type ChatProjectTemplatesSettings = Required<
	Pick<CreateChatProjectRequest, "limit_templates" | "template_ids">
>;

/** Returns the form state for saved templates, or an empty form when none. */
export const chatProjectTemplatesValues = (
	templates: ChatProjectTemplatesSettings | undefined,
): ChatProjectTemplatesValues => ({
	limitTemplates: templates?.limit_templates ?? false,
	templateIds: templates?.template_ids ?? [],
});

/**
 * Builds the request from the form values. Templates that are no longer
 * eligible (deleted, deprecated, or no longer allowed for agents) are
 * dropped, since the server would reject them.
 */
export const chatProjectTemplatesRequest = (
	values: ChatProjectTemplatesValues,
	eligible: readonly Template[],
): ChatProjectTemplatesSettings => ({
	limit_templates: values.limitTemplates,
	template_ids: values.templateIds.filter((id) =>
		eligible.some((template) => template.id === id),
	),
});

/**
 * Returns the server's rejection of the templates in a save error. Forms
 * have no field named after these validations, so the templates fields show
 * it through `saveError`.
 */
export const templatesValidationDetail = (error: unknown) =>
	isApiValidationError(error)
		? error.response.data.validations?.find(
				(validation) => validation.field === "template_ids",
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
 * Fields that pick a project's templates and whether its chats are limited to
 * them.
 */
export const ChatProjectTemplatesFields: React.FC<
	ChatProjectTemplatesFieldsProps
> = ({ templates, templatesError, saveError, value, onChange, disabled }) => {
	const limitsToNothing =
		value.limitTemplates && value.templateIds.length === 0;
	const options = (templates ?? []).map((template) => ({
		value: template.id,
		label: templateLabel(template),
	}));

	const setTemplateIds = (templateIds: readonly string[]) => {
		onChange({
			...value,
			templateIds,
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
			{saveError && (
				<p role="alert" className="m-0 text-sm text-content-destructive">
					{saveError}
				</p>
			)}
		</>
	);
};
