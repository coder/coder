import { useState } from "react";
import { useQuery } from "react-query";
import { isApiValidationError } from "#/api/errors";
import { chatProjects } from "#/api/queries/chatProjects";
import {
	agentTemplates,
	richParameters,
	templateVersionPresets,
} from "#/api/queries/templates";
import type { ChatProject, ChatProjectTemplates } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	ChatProjectTemplatesFields,
	type ChatProjectTemplatesValues,
	chatProjectTemplatesRequest,
	chatProjectTemplatesValues,
	templatesValidationDetail,
} from "../ChatsSidebar/dialogs/ChatProjectTemplatesFields";
import { ProjectEditConflictAlert } from "./ProjectEditConflictAlert";

type ProjectTemplatesDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSaving: boolean;
	readonly error: unknown;
	readonly onSave: (templates: ChatProjectTemplates) => void;
	/** Clears `error`, for when the draft is replaced by the latest save. */
	readonly onResetError: () => void;
	/**
	 * Called when focus would return after closing. The dialog has no
	 * trigger, so without this focus lands on the document body.
	 */
	readonly onCloseAutoFocus?: (event: Event) => void;
};

/** Edits the templates a project's chats may use, and their defaults. */
export const ProjectTemplatesDialog: React.FC<ProjectTemplatesDialogProps> = ({
	open,
	onOpenChange,
	onCloseAutoFocus,
	...formProps
}) => {
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !formProps.isSaving) {
			onOpenChange(false);
		}
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent
				aria-describedby={undefined}
				onCloseAutoFocus={onCloseAutoFocus}
			>
				<ProjectTemplatesForm
					{...formProps}
					open={open}
					onCancel={() => handleOpenChange(false)}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ProjectTemplatesFormProps = Omit<
	ProjectTemplatesDialogProps,
	"onOpenChange" | "onCloseAutoFocus"
> & {
	readonly onCancel: () => void;
};

/**
 * Draft and server copies can list `default_parameters` keys in different
 * orders, so keys are sorted before comparing.
 */
const serializeTemplates = (templates: ChatProjectTemplates) =>
	JSON.stringify([
		templates.template_ids,
		templates.default_template_id ?? "",
		templates.default_preset_id ?? "",
		Object.entries(templates.default_parameters ?? {}).sort(([a], [b]) =>
			a.localeCompare(b),
		),
	]);

/**
 * An empty parameter input means "use the template default", so the request
 * omits it. An empty value that is already saved (set through the API) is
 * kept while its input stays empty, so editing other settings does not drop
 * it.
 */
const keepSavedEmptyParameters = (
	request: ChatProjectTemplates,
	value: ChatProjectTemplatesValues,
	saved: ChatProjectTemplates,
): ChatProjectTemplates => {
	if (
		value.defaultsKind !== "parameters" ||
		request.default_template_id === undefined ||
		request.default_template_id !== saved.default_template_id
	) {
		return request;
	}
	const savedEmpty = Object.entries(saved.default_parameters ?? {}).filter(
		([name, savedValue]) =>
			savedValue === "" && value.defaultParameters[name] === "",
	);
	if (savedEmpty.length === 0) {
		return request;
	}
	return {
		...request,
		default_parameters: {
			...request.default_parameters,
			...Object.fromEntries(savedEmpty),
		},
	};
};

const ProjectTemplatesForm: React.FC<ProjectTemplatesFormProps> = ({
	open,
	project,
	isSaving,
	error,
	onSave,
	onResetError,
	onCancel,
}) => {
	// The dialog content mounts on open, so the draft starts from the
	// templates saved at that time and a project refetch does not reset it.
	const [baseTemplates, setBaseTemplates] = useState(project.templates);
	const initialValue = chatProjectTemplatesValues(baseTemplates);
	const [value, setValue] = useState(initialValue);
	// Templates are shared by every editor of the project, so refresh them
	// on open and when the tab regains focus to detect changes made elsewhere.
	useQuery({ ...chatProjects(), refetchOnWindowFocus: true });
	const isDraftUntouched =
		JSON.stringify(value) === JSON.stringify(initialValue);
	if (
		isDraftUntouched &&
		serializeTemplates(project.templates) !== serializeTemplates(baseTemplates)
	) {
		setBaseTemplates(project.templates);
		setValue(chatProjectTemplatesValues(project.templates));
	}
	const templatesQuery = useQuery(agentTemplates(project.organization_id));
	const eligibleTemplates = templatesQuery.data;
	const defaultTemplate = eligibleTemplates?.find(
		(template) => template.id === value.defaultTemplateId,
	);
	const hasDefaultPreset =
		value.defaultsKind === "preset" && value.defaultPresetId !== "";
	const presetsQuery = useQuery({
		...templateVersionPresets(defaultTemplate?.active_version_id ?? ""),
		enabled: defaultTemplate !== undefined && hasDefaultPreset,
	});
	// A new template version can drop the saved default preset. Chats skip
	// it, and the server rejects saving it again, so the form clears it.
	const hasStalePreset =
		hasDefaultPreset &&
		presetsQuery.data !== undefined &&
		!(presetsQuery.data ?? []).some(
			(preset) => preset.ID === value.defaultPresetId,
		);
	const hasDefaultParameters =
		value.defaultsKind === "parameters" &&
		Object.keys(value.defaultParameters).length > 0;
	const parametersQuery = useQuery({
		...richParameters(defaultTemplate?.active_version_id ?? ""),
		enabled: defaultTemplate !== undefined && hasDefaultParameters,
	});
	// Values for parameters the active version no longer has are not shown,
	// and the server rejects them for classic templates, so the form drops
	// them.
	const parameterNames = parametersQuery.data?.map(({ name }) => name);
	const staleParameterNames =
		hasDefaultParameters && parameterNames
			? Object.keys(value.defaultParameters).filter(
					(name) => !parameterNames.includes(name),
				)
			: [];
	const formValue: ChatProjectTemplatesValues = {
		...value,
		defaultPresetId: hasStalePreset ? "" : value.defaultPresetId,
		defaultParameters: Object.fromEntries(
			Object.entries(value.defaultParameters).filter(
				([name]) => !staleParameterNames.includes(name),
			),
		),
	};
	const request =
		eligibleTemplates &&
		keepSavedEmptyParameters(
			chatProjectTemplatesRequest(formValue, eligibleTemplates),
			formValue,
			baseTemplates,
		);
	// Saving drops templates that are no longer eligible, so a draft that
	// still holds any differs from the saved templates even when untouched.
	const droppedTemplateCount =
		request === undefined
			? 0
			: value.templateIds.length - request.template_ids.length;
	const isUnchanged =
		droppedTemplateCount === 0 &&
		!hasStalePreset &&
		staleParameterNames.length === 0 &&
		isDraftUntouched;
	// Save replaces every template setting, so a change saved elsewhere must
	// be resolved first. Templates that already match the draft, such as this
	// dialog's own save arriving before it closes, are not a conflict.
	const serializedLatest = serializeTemplates(project.templates);
	const hasConflict =
		open &&
		serializedLatest !== serializeTemplates(baseTemplates) &&
		(request === undefined || serializedLatest !== serializeTemplates(request));
	const canSave =
		request !== undefined && !isUnchanged && !hasConflict && !isSaving;

	return (
		<form
			className="flex flex-col gap-4"
			onSubmit={(event) => {
				event.preventDefault();
				if (request && canSave) {
					onSave(request);
				}
			}}
		>
			<DialogHeader>
				<DialogTitle>Edit templates</DialogTitle>
			</DialogHeader>
			<ChatProjectTemplatesFields
				templates={eligibleTemplates}
				templatesError={templatesQuery.error}
				saveError={templatesValidationDetail(error)}
				value={formValue}
				onChange={(next) => {
					setValue(next);
					if (error) {
						onResetError();
					}
				}}
				disabled={isSaving}
			/>
			{(droppedTemplateCount > 0 ||
				hasStalePreset ||
				staleParameterNames.length > 0) && (
				<Alert severity="warning">
					{droppedTemplateCount > 0 && (
						<AlertDescription>
							{droppedTemplateCount === 1
								? "1 template is"
								: `${droppedTemplateCount} templates are`}{" "}
							no longer available to agents (deleted, deprecated, or no longer
							allowed for agents). Saving removes{" "}
							{droppedTemplateCount === 1 ? "it" : "them"} from the project.
						</AlertDescription>
					)}
					{staleParameterNames.length > 0 && (
						<AlertDescription>
							The template's active version no longer has{" "}
							{staleParameterNames.join(", ")}. Saving removes{" "}
							{staleParameterNames.length === 1 ? "its value" : "their values"}.
						</AlertDescription>
					)}
					{hasStalePreset && (
						<AlertDescription>
							The default preset is no longer in the template's active version,
							so chats do not use it. Choose another preset, or save to clear
							it.
						</AlertDescription>
					)}
				</Alert>
			)}
			{hasConflict && (
				<ProjectEditConflictAlert
					disabled={isSaving}
					onKeepDraft={() => setBaseTemplates(project.templates)}
					onLoadLatest={() => {
						setBaseTemplates(project.templates);
						setValue(chatProjectTemplatesValues(project.templates));
						onResetError();
					}}
				>
					Someone else changed these templates while you were editing. Load the
					latest templates, or keep your draft to replace them.
				</ProjectEditConflictAlert>
			)}
			{Boolean(error) && !isApiValidationError(error) && (
				<ErrorAlert error={error} showDebugDetail={false} />
			)}
			<DialogFooter>
				<Button
					type="button"
					variant="outline"
					disabled={isSaving}
					onClick={onCancel}
				>
					Cancel
				</Button>
				<Button type="submit" disabled={!canSave}>
					<Spinner loading={isSaving} />
					Save
				</Button>
			</DialogFooter>
		</form>
	);
};
