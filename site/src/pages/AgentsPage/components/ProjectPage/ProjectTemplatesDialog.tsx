import { useState } from "react";
import { useQuery } from "react-query";
import { isApiValidationError } from "#/api/errors";
import { agentTemplates } from "#/api/queries/templates";
import type { ChatProject, ChatProjectTemplates } from "#/api/typesGenerated";
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
	const templatesQuery = useQuery(agentTemplates(project.organization_id));
	const eligibleTemplates = templatesQuery.data;
	const request =
		eligibleTemplates && chatProjectTemplatesRequest(value, eligibleTemplates);
	// Saving drops templates that are no longer eligible, so a draft that
	// still holds any differs from the saved templates even when untouched.
	const dropsIneligible =
		request !== undefined &&
		request.template_ids.length < value.templateIds.length;
	const isUnchanged =
		!dropsIneligible && JSON.stringify(value) === JSON.stringify(initialValue);
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
				value={value}
				onChange={setValue}
				disabled={isSaving}
			/>
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
