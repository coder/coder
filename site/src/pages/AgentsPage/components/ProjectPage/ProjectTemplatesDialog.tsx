import { useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { isApiValidationError } from "#/api/errors";
import type { ChatProjectTemplatesChange } from "#/api/queries/chatProjects";
import { agentTemplates } from "#/api/queries/templates";
import type { ChatProject } from "#/api/typesGenerated";
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
import {
	savedTemplates,
	serializeTemplates,
	templatesChange,
} from "./projectTemplatesDraft";

type ProjectTemplatesDialogProps = {
	readonly project: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSaving: boolean;
	readonly error: unknown;
	readonly onSave: (change: ChatProjectTemplatesChange) => void;
	readonly onResetError: () => void;
	/**
	 * Called when focus would return after closing. The dialog has no
	 * trigger, so without this focus lands on the document body.
	 */
	readonly onCloseAutoFocus?: (event: Event) => void;
};

/**
 * Edits a project's templates, whether its chats are limited to them, and
 * the default template.
 */
export const ProjectTemplatesDialog: React.FC<ProjectTemplatesDialogProps> = ({
	open,
	onOpenChange,
	onCloseAutoFocus,
	...formProps
}) => {
	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen || formProps.isSaving) {
			return;
		}
		onOpenChange(false);
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent
				aria-describedby={undefined}
				onCloseAutoFocus={onCloseAutoFocus}
			>
				<ProjectTemplatesForm
					{...formProps}
					onCancel={() => handleOpenChange(false)}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ProjectTemplatesFormProps = Omit<
	ProjectTemplatesDialogProps,
	"open" | "onOpenChange" | "onCloseAutoFocus"
> & {
	readonly onCancel: () => void;
};

const ProjectTemplatesForm: React.FC<ProjectTemplatesFormProps> = ({
	project,
	isSaving,
	error: shownError,
	onSave,
	onResetError,
	onCancel,
}) => {
	const latestTemplates = savedTemplates(project);
	// The dialog content mounts on open, so the draft starts from the
	// templates saved at that time and a project refetch does not reset it.
	const [baseTemplates] = useState(latestTemplates);
	const initialValue = chatProjectTemplatesValues(baseTemplates);
	const [value, setValue] = useState(initialValue);
	// Save keeps focus while the dialog closes after a successful save, so
	// a repeated Enter must not send the same save again. Only a failed save
	// allows another.
	const [isSaveSent, setIsSaveSent] = useState(false);
	// After a save that leaves the dialog open, focus moves to Save, but only
	// when focus was lost, such as inputs disabled during a save.
	const pendingFocusRef = useRef<"save" | null>(null);
	const saveButtonRef = useRef<HTMLButtonElement>(null);
	if (isSaveSent && shownError) {
		setIsSaveSent(false);
	}
	const templatesQuery = useQuery(agentTemplates(project.organization_id));
	const eligibleTemplates = templatesQuery.data;
	const request =
		eligibleTemplates && chatProjectTemplatesRequest(value, eligibleTemplates);
	const baseRequest =
		eligibleTemplates &&
		chatProjectTemplatesRequest(initialValue, eligibleTemplates);
	// A request leaves out templates that are no longer eligible, so a draft
	// that only selects one of those has nothing to save.
	const hasChanges =
		request !== undefined &&
		baseRequest !== undefined &&
		serializeTemplates(request) !== serializeTemplates(baseRequest);
	const isBusy = isSaving || templatesQuery.isFetching;
	const canSave = hasChanges && !isSaveSent && !isSaving;
	useEffect(() => {
		const pending = pendingFocusRef.current;
		// Wait for the outcome: a save either closes the dialog or fails.
		if (isBusy || (pending === "save" && !shownError)) {
			return;
		}
		pendingFocusRef.current = null;
		const focused = document.activeElement;
		// The dialog container takes focus after a click on empty space, so it
		// only counts as lost focus when an outcome is waiting to be shown.
		const isFocusLost =
			!focused ||
			focused === document.body ||
			focused === saveButtonRef.current ||
			(pending !== null && focused.getAttribute("role") === "dialog");
		if (!isFocusLost) {
			return;
		}
		if (pending !== null || canSave) {
			// Save stays focusable while it waits.
			saveButtonRef.current?.focus();
		}
	}, [isBusy, canSave, shownError]);

	return (
		<form
			className="flex flex-col gap-4"
			// The parameter fields carry the template's constraints, such as
			// required, which the browser would enforce on submit. A project
			// default may leave a required parameter empty.
			noValidate
			onSubmit={(event) => {
				event.preventDefault();
				if (!request || !eligibleTemplates || !canSave) {
					return;
				}
				// The previous failure's error would otherwise unblock Save again
				// while this save runs.
				if (shownError) {
					onResetError();
				}
				// A rejected save leaves the dialog open, so focus returns to Save.
				pendingFocusRef.current = "save";
				setIsSaveSent(true);
				// The saved templates pass the draft's eligibility filter, so a
				// template that left the list of eligible templates is left alone.
				onSave(
					templatesChange(
						chatProjectTemplatesRequest(
							chatProjectTemplatesValues(latestTemplates),
							eligibleTemplates,
						),
						request,
					),
				);
			}}
		>
			<DialogHeader>
				<DialogTitle>Edit templates</DialogTitle>
			</DialogHeader>
			<ChatProjectTemplatesFields
				templates={eligibleTemplates}
				templatesError={templatesQuery.error}
				saveError={templatesValidationDetail(shownError)}
				value={value}
				hiddenTemplateCount={project.hidden_template_count}
				onChange={(next) => {
					setValue(next);
					if (shownError) {
						onResetError();
					}
				}}
				disabled={isSaving}
			/>
			{Boolean(shownError) && !isApiValidationError(shownError) && (
				<ErrorAlert error={shownError} showDebugDetail={false} />
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
				<Button
					ref={saveButtonRef}
					type="submit"
					// Unlike disabled, aria-disabled keeps focus on Save while it
					// waits, so Tab cannot leave the dialog. Submit checks canSave.
					aria-disabled={!canSave}
					aria-busy={isBusy}
				>
					<Spinner loading={isBusy} />
					Save
				</Button>
			</DialogFooter>
		</form>
	);
};
