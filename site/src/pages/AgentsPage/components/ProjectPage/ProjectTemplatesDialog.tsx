import { useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { isApiValidationError } from "#/api/errors";
import {
	type ChatProjectTemplatesChange,
	chatProjects,
} from "#/api/queries/chatProjects";
import { agentTemplates } from "#/api/queries/templates";
import type { ChatProject } from "#/api/typesGenerated";
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
	chatProjectTemplatesRequest,
	chatProjectTemplatesValues,
	templatesValidationDetail,
} from "../ChatsSidebar/dialogs/ChatProjectTemplatesFields";
import { ProjectEditConflictAlert } from "./ProjectEditConflictAlert";
import {
	rebaseValues,
	savedTemplates,
	serializeTemplates,
	serializeValues,
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

const ProjectTemplatesForm: React.FC<ProjectTemplatesFormProps> = ({
	open,
	project,
	isSaving,
	error,
	onSave,
	onResetError,
	onCancel,
}) => {
	const latestTemplates = savedTemplates(project);
	// The dialog content mounts on open, so the draft starts from the
	// templates saved at that time and a project refetch does not reset it.
	const [baseTemplates, setBaseTemplates] = useState(latestTemplates);
	const initialValue = chatProjectTemplatesValues(baseTemplates);
	const [value, setValue] = useState(initialValue);
	// Save keeps focus while the dialog closes after a successful save, so
	// a repeated Enter must not send the same save again. Only a failed save
	// allows another.
	const [isSaveSent, setIsSaveSent] = useState(false);
	// Outcomes of a save or refresh that leaves the dialog open: focus moves
	// to what explains it (Retry or Save), but only
	// when focus was lost, such as inputs disabled during a save.
	const pendingFocusRef = useRef<"retry" | "save" | null>(null);
	const retryButtonRef = useRef<HTMLButtonElement>(null);
	const saveButtonRef = useRef<HTMLButtonElement>(null);
	// A rejected save no longer applies once the draft follows newer
	// templates, so its error is hidden.
	const [dismissedError, setDismissedError] = useState<unknown>();
	const shownError = error === dismissedError ? undefined : error;
	if (isSaveSent && shownError) {
		setIsSaveSent(false);
	}
	// Templates are shared by every editor of the project, so refresh them
	// on open and when the tab regains focus to detect changes made elsewhere.
	const projectsQuery = useQuery({
		...chatProjects(),
		refetchOnWindowFocus: true,
	});
	const isDraftUntouched =
		serializeValues(value) === serializeValues(initialValue);
	const serializedLatest = serializeTemplates(latestTemplates);
	const isLatestNew = serializedLatest !== serializeTemplates(baseTemplates);
	// An untouched draft follows newer templates. So does a draft after its
	// own save changed the project, even when that save failed partway: it
	// keeps its edits, and saving again sends what is left of them.
	const followLatest = () => {
		setBaseTemplates(latestTemplates);
		setValue(
			rebaseValues(
				value,
				initialValue,
				chatProjectTemplatesValues(latestTemplates),
			),
		);
	};
	if (isLatestNew && (isDraftUntouched || isSaveSent)) {
		if (isDraftUntouched) {
			setDismissedError(error);
		}
		followLatest();
	}
	const templatesQuery = useQuery({
		...agentTemplates(project.organization_id),
		refetchOnWindowFocus: true,
	});
	const eligibleTemplates = templatesQuery.data;
	const request =
		eligibleTemplates && chatProjectTemplatesRequest(value, eligibleTemplates);
	// A change saved elsewhere can touch what the draft changes, so the user
	// resolves it first. Templates that already match the draft, such as this
	// dialog's own save arriving before it closes, are not a conflict.
	const hasConflict =
		open &&
		isLatestNew &&
		(request === undefined || serializedLatest !== serializeTemplates(request));
	// Without a fresh copy of the project, Save could overwrite a limit or
	// default template changed elsewhere that the conflict check has not seen.
	const failedQueries = [projectsQuery, templatesQuery].filter(
		(query) => query.isError,
	);
	const refreshFailed = failedQueries.length > 0;
	// Retry stays enabled while it runs so it keeps focus.
	const isRetrying = failedQueries.some((query) => query.isFetching);
	// A stale list can miss a template's new active version.
	const isRefreshing = projectsQuery.isFetching || templatesQuery.isFetching;
	const isBusy =
		isSaving || projectsQuery.isFetching || templatesQuery.isFetching;
	const canSave =
		request !== undefined &&
		!isDraftUntouched &&
		!hasConflict &&
		!refreshFailed &&
		!isRefreshing &&
		!isSaveSent &&
		!isSaving;
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
			focused === retryButtonRef.current ||
			(pending !== null && focused.getAttribute("role") === "dialog");
		if (!isFocusLost) {
			return;
		}
		if (pending === "retry" && refreshFailed) {
			retryButtonRef.current?.focus();
		} else if (pending !== null || canSave) {
			// Save stays focusable while it waits. This also covers Save losing
			// focus to a background refresh.
			saveButtonRef.current?.focus();
		}
	}, [isBusy, refreshFailed, canSave, shownError]);

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
			{refreshFailed && (
				<Alert
					severity="error"
					actions={
						<Button
							ref={retryButtonRef}
							type="button"
							size="sm"
							variant="outline"
							aria-busy={isRetrying}
							onClick={() => {
								if (isRetrying) {
									return;
								}
								pendingFocusRef.current = "retry";
								for (const query of failedQueries) {
									void query.refetch();
								}
							}}
						>
							<Spinner loading={isRetrying} />
							Retry
						</Button>
					}
				>
					<AlertDescription>
						Couldn't load the latest templates, so saving is paused.
					</AlertDescription>
				</Alert>
			)}
			{hasConflict && (
				<ProjectEditConflictAlert
					disabled={isSaving}
					// The warning unmounts once resolved, so focus moves to Save.
					onKeepDraft={() => {
						followLatest();
						saveButtonRef.current?.focus();
					}}
					onLoadLatest={() => {
						setBaseTemplates(latestTemplates);
						setValue(chatProjectTemplatesValues(latestTemplates));
						onResetError();
						// The newer save can include templates this list hasn't seen.
						void templatesQuery.refetch();
						saveButtonRef.current?.focus();
					}}
				>
					Someone else changed these templates while you were editing. Load the
					latest templates, or keep your draft to apply your changes to them.
				</ProjectEditConflictAlert>
			)}
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
