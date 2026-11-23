import { useEffect, useRef, useState } from "react";
import {
	type QueryClient,
	type QueryKey,
	useQuery,
	useQueryClient,
} from "react-query";
import { isApiValidationError } from "#/api/errors";
import {
	type ChatProjectTemplatesChange,
	chatProjects,
	chatProjectsKey,
} from "#/api/queries/chatProjects";
import {
	agentTemplates,
	agentTemplatesKey,
	richParameters,
	templateVersionPresets,
} from "#/api/queries/templates";
import type { ChatProject, Template } from "#/api/typesGenerated";
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
	const queryClient = useQueryClient();
	// Identifies the latest pre-save check. Closing the dialog or leaving the
	// page moves it on, so a check that resolves later does not save.
	const checkIdRef = useRef(0);
	useEffect(
		() => () => {
			checkIdRef.current += 1;
		},
		[],
	);
	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen || formProps.isSaving) {
			return;
		}
		checkIdRef.current += 1;
		// A hung check would otherwise keep the next open dialog waiting.
		replaceRequests(queryClient, [
			chatProjectsKey,
			agentTemplatesKey(formProps.project.organization_id),
		]);
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
					onBeginCheck={() => {
						checkIdRef.current += 1;
						return checkIdRef.current;
					}}
					isCheckCurrent={(checkId) => checkId === checkIdRef.current}
					onCancel={() => handleOpenChange(false)}
				/>
			</DialogContent>
		</Dialog>
	);
};

/**
 * Cancels the requests in flight for these queries. Other views share the
 * queries, so fresh requests replace them.
 */
const replaceRequests = (
	queryClient: QueryClient,
	queryKeys: readonly QueryKey[],
) => {
	for (const queryKey of queryKeys) {
		const filters = { queryKey, exact: true };
		void queryClient
			.cancelQueries(filters)
			.then(() => queryClient.invalidateQueries(filters));
	}
};

type ProjectTemplatesFormProps = Omit<
	ProjectTemplatesDialogProps,
	"onOpenChange" | "onCloseAutoFocus"
> & {
	readonly onBeginCheck: () => number;
	/** False once the dialog closed after the check began. */
	readonly isCheckCurrent: (checkId: number) => boolean;
	readonly onCancel: () => void;
};

/**
 * Covers what a request for the draft's templates depends on: which of them
 * are eligible, their active versions, and their parameter mode.
 */
const serializeEligibleTemplates = (
	templates: readonly Template[] | undefined,
	draftTemplateIds: readonly string[],
) =>
	JSON.stringify(
		(templates ?? [])
			.filter(({ id }) => draftTemplateIds.includes(id))
			.map(
				({ id, active_version_id, use_classic_parameter_flow }) =>
					`${id}:${active_version_id}:${use_classic_parameter_flow}`,
			)
			.sort(),
	);

const ProjectTemplatesForm: React.FC<ProjectTemplatesFormProps> = ({
	onBeginCheck,
	isCheckCurrent,
	open,
	project,
	isSaving,
	error,
	onSave,
	onResetError,
	onCancel,
}) => {
	const queryClient = useQueryClient();
	const latestTemplates = savedTemplates(project);
	// The dialog content mounts on open, so the draft starts from the
	// templates saved at that time and a project refetch does not reset it.
	const [baseTemplates, setBaseTemplates] = useState(latestTemplates);
	const initialValue = chatProjectTemplatesValues(baseTemplates);
	const [value, setValue] = useState(initialValue);
	// Set before the pre-save check renders, so a second submit in the same
	// moment cannot cancel that check's request and save unchecked.
	const isSubmittingRef = useRef(false);
	const [isCheckingBeforeSave, setIsCheckingBeforeSave] = useState(false);
	// Save keeps focus while the dialog closes after a successful save, so
	// a repeated Enter must not send the same save again. Only a failed save
	// allows another.
	const [isSaveSent, setIsSaveSent] = useState(false);
	// Outcomes of a save or refresh that leaves the dialog open: focus moves
	// to what explains it (the conflict warning, Retry, or Save), but only
	// when focus was lost, such as inputs disabled during a save.
	const pendingFocusRef = useRef<"conflict" | "retry" | "save" | null>(null);
	const conflictAlertRef = useRef<HTMLDivElement>(null);
	const retryButtonRef = useRef<HTMLButtonElement>(null);
	const saveButtonRef = useRef<HTMLButtonElement>(null);
	// Counts draft edits. The draft stays editable during the pre-save
	// check, so a check that sees a newer edit saves nothing.
	const draftVersionRef = useRef(0);
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
	// The default template's active version decides which saved defaults
	// are stale, so the list refreshes with the project.
	const templatesQuery = useQuery({
		...agentTemplates(project.organization_id),
		refetchOnWindowFocus: true,
	});
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
	// Only classic templates have a fixed parameter list. Dynamic templates
	// can add parameters during evaluation, which the static list misses.
	const hasDefaultParameters =
		value.defaultsKind === "parameters" &&
		defaultTemplate?.use_classic_parameter_flow === true &&
		Object.keys(value.defaultParameters).length > 0;
	const parametersQuery = useQuery({
		...richParameters(defaultTemplate?.active_version_id ?? ""),
		enabled: defaultTemplate !== undefined && hasDefaultParameters,
	});
	const defaultsVersionId = defaultTemplate?.active_version_id;
	// A stalled request for the defaults would keep the next open dialog
	// waiting too, so this form replaces its requests when it unmounts.
	useEffect(() => {
		if (defaultsVersionId === undefined) {
			return;
		}
		return () =>
			replaceRequests(queryClient, [
				templateVersionPresets(defaultsVersionId).queryKey,
				richParameters(defaultsVersionId).queryKey,
			]);
	}, [queryClient, defaultsVersionId]);
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
		chatProjectTemplatesRequest(formValue, eligibleTemplates);
	const baseRequest =
		eligibleTemplates &&
		chatProjectTemplatesRequest(initialValue, eligibleTemplates);
	// A request leaves out templates that are no longer eligible, so a draft
	// that only selects one of those has nothing to save. An untouched draft
	// with a stale default does, because saving clears that default.
	const hasChanges =
		request !== undefined &&
		baseRequest !== undefined &&
		serializeTemplates(request) !== serializeTemplates(baseRequest);
	// A change saved elsewhere can touch what the draft changes, so the user
	// resolves it first. Templates that already match the draft, such as this
	// dialog's own save arriving before it closes, are not a conflict.
	const hasConflict =
		open &&
		isLatestNew &&
		(request === undefined || serializedLatest !== serializeTemplates(request));
	// Without a fresh copy of the project, Save could overwrite a limit or
	// default template changed elsewhere that the conflict check has not seen.
	// The defaults queries decide which stale values the save clears.
	// A disabled query keeps its last error, so only the queries this draft
	// uses count.
	const failedQueries = [
		projectsQuery,
		templatesQuery,
		...(hasDefaultPreset ? [presetsQuery] : []),
		...(hasDefaultParameters ? [parametersQuery] : []),
	].filter((query) => query.isError);
	const refreshFailed = failedQueries.length > 0;
	// Retry stays enabled while it runs so it keeps focus.
	const isRetrying = failedQueries.some((query) => query.isFetching);
	const isLoadingDefaults =
		(hasDefaultPreset && presetsQuery.isFetching) ||
		(hasDefaultParameters && parametersQuery.isFetching);
	// A stale list can miss a template's new active version. The pre-save
	// check's own refresh leaves Save enabled, so focus stays on it.
	const isRefreshing =
		(projectsQuery.isFetching && !isCheckingBeforeSave) ||
		templatesQuery.isFetching ||
		isLoadingDefaults;
	const isBusy =
		isSaving ||
		isCheckingBeforeSave ||
		projectsQuery.isFetching ||
		templatesQuery.isFetching ||
		isLoadingDefaults;
	const canSave =
		hasChanges &&
		!hasConflict &&
		!refreshFailed &&
		!isRefreshing &&
		!isSaveSent &&
		!isSaving;
	useEffect(() => {
		const pending = pendingFocusRef.current;
		// Wait for the outcome: a save either closes the dialog or fails, and
		// a found conflict renders its warning after the project refreshes.
		if (
			isBusy ||
			(pending === "save" && !shownError) ||
			(pending === "conflict" && !hasConflict)
		) {
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
		if (pending === "conflict") {
			conflictAlertRef.current?.focus();
		} else if (pending === "retry" && refreshFailed) {
			retryButtonRef.current?.focus();
		} else if (pending !== null || canSave) {
			// Save stays focusable while it waits. This also covers Save losing
			// focus to a background refresh.
			saveButtonRef.current?.focus();
		}
	}, [isBusy, hasConflict, refreshFailed, canSave, shownError]);

	return (
		<form
			className="flex flex-col gap-4"
			// The parameter fields carry the template's constraints, such as
			// required, which the browser would enforce on submit. A project
			// default may leave a required parameter empty.
			noValidate
			onSubmit={async (event) => {
				event.preventDefault();
				if (
					!request ||
					!eligibleTemplates ||
					!canSave ||
					isSubmittingRef.current
				) {
					return;
				}
				isSubmittingRef.current = true;
				// The previous failure's error would otherwise unblock Save again
				// while this save runs.
				if (shownError) {
					onResetError();
				}
				// A change saved elsewhere since the last refresh would otherwise
				// be overwritten, so check first. A newer save shows the conflict
				// warning instead.
				const checkId = onBeginCheck();
				const draftVersion = draftVersionRef.current;
				setIsCheckingBeforeSave(true);
				// The request was built from the eligible-templates list, so it is
				// refreshed too.
				const [refreshed, refreshedTemplates] = await Promise.all([
					projectsQuery.refetch(),
					templatesQuery.refetch(),
				]);
				isSubmittingRef.current = false;
				setIsCheckingBeforeSave(false);
				if (
					!isCheckCurrent(checkId) ||
					draftVersion !== draftVersionRef.current
				) {
					return;
				}
				if (refreshed.isError || refreshedTemplates.isError) {
					pendingFocusRef.current = "retry";
					return;
				}
				// A changed list rebuilds the request; the user saves again.
				if (
					serializeEligibleTemplates(
						refreshedTemplates.data,
						value.templateIds,
					) !== serializeEligibleTemplates(eligibleTemplates, value.templateIds)
				) {
					return;
				}
				const latest = refreshed.data?.find(({ id }) => id === project.id);
				if (!latest) {
					return;
				}
				const saved = savedTemplates(latest);
				const serializedSaved = serializeTemplates(saved);
				if (
					serializedSaved !== serializeTemplates(baseTemplates) &&
					serializedSaved !== serializeTemplates(request)
				) {
					pendingFocusRef.current = "conflict";
					return;
				}
				// A rejected save leaves the dialog open, so focus returns to Save.
				pendingFocusRef.current = "save";
				setIsSaveSent(true);
				// The saved templates pass the draft's eligibility filter, so a
				// template that left the list of eligible templates is left alone.
				onSave(
					templatesChange(
						chatProjectTemplatesRequest(
							chatProjectTemplatesValues(saved),
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
				value={formValue}
				hiddenTemplateCount={project.hidden_template_count}
				onChange={(next) => {
					draftVersionRef.current += 1;
					setValue(next);
					if (shownError) {
						onResetError();
					}
				}}
				disabled={isSaving}
			/>
			{(hasStalePreset || staleParameterNames.length > 0) && (
				<Alert severity="warning">
					{staleParameterNames.length > 0 && (
						<AlertDescription className="block">
							The template's active version no longer has{" "}
							{staleParameterNames.join(", ")}. Saving removes{" "}
							{staleParameterNames.length === 1 ? "its value" : "their values"}.
						</AlertDescription>
					)}
					{hasStalePreset && (
						<AlertDescription className="block">
							The default preset is no longer in the template's active version,
							so chats do not use it. Choose another preset, or save to clear
							it.
						</AlertDescription>
					)}
				</Alert>
			)}
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
						Couldn't load the latest templates and their defaults, so saving is
						paused.
					</AlertDescription>
				</Alert>
			)}
			{hasConflict && (
				// Focus lands on the warning rather than Load latest, so a repeated
				// Enter cannot discard the draft.
				<div
					ref={conflictAlertRef}
					tabIndex={-1}
					className="rounded-md outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					<ProjectEditConflictAlert
						disabled={isSaving}
						// The warning unmounts once resolved, so focus moves to Save.
						onKeepDraft={() => {
							draftVersionRef.current += 1;
							followLatest();
							saveButtonRef.current?.focus();
						}}
						onLoadLatest={() => {
							draftVersionRef.current += 1;
							setBaseTemplates(latestTemplates);
							setValue(chatProjectTemplatesValues(latestTemplates));
							onResetError();
							// The newer save can include templates this list hasn't seen.
							void templatesQuery.refetch();
							saveButtonRef.current?.focus();
						}}
					>
						Someone else changed these templates while you were editing. Load
						the latest templates, or keep your draft to apply your changes to
						them.
					</ProjectEditConflictAlert>
				</div>
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
