import { useEffect, useRef, useState } from "react";
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
	const [isCheckingBeforeSave, setIsCheckingBeforeSave] = useState(false);
	// Identifies the latest pre-save check. Closing the dialog or leaving the
	// page moves it on, so a check that resolves later does not save.
	const checkIdRef = useRef(0);
	useEffect(
		() => () => {
			checkIdRef.current += 1;
		},
		[],
	);
	const beginCheck = () => {
		checkIdRef.current += 1;
		setIsCheckingBeforeSave(true);
		return checkIdRef.current;
	};
	const endCheck = (checkId: number) => {
		if (checkId !== checkIdRef.current) {
			return false;
		}
		setIsCheckingBeforeSave(false);
		return true;
	};
	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen || formProps.isSaving) {
			return;
		}
		checkIdRef.current += 1;
		setIsCheckingBeforeSave(false);
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
					isCheckingBeforeSave={isCheckingBeforeSave}
					onBeginCheck={beginCheck}
					onEndCheck={endCheck}
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
	readonly isCheckingBeforeSave: boolean;
	readonly onBeginCheck: () => number;
	/** Ends a check; false when it was cancelled and must not save. */
	readonly onEndCheck: (checkId: number) => boolean;
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

/** Serializes form values so parameter order does not matter. */
const serializeValues = (values: ChatProjectTemplatesValues) =>
	JSON.stringify({
		...values,
		defaultParameters: Object.entries(values.defaultParameters).sort(
			([a], [b]) => a.localeCompare(b),
		),
	});

const ProjectTemplatesForm: React.FC<ProjectTemplatesFormProps> = ({
	isCheckingBeforeSave,
	onBeginCheck,
	onEndCheck,
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
	// Set before the pre-save check renders, so a second submit in the same
	// moment cannot cancel that check's request and save unchecked.
	const isSubmittingRef = useRef(false);
	// The check disables the focused control, which drops focus to the
	// page. When it ends without saving, focus moves to the control that
	// explains why, once that control renders enabled.
	const pendingFocusRef = useRef<"loadLatest" | "retry" | "save" | null>(null);
	const loadLatestButtonRef = useRef<HTMLButtonElement>(null);
	const retryButtonRef = useRef<HTMLButtonElement>(null);
	const saveButtonRef = useRef<HTMLButtonElement>(null);
	const isBusy = isSaving || isCheckingBeforeSave;
	// A rejected save no longer applies once the draft follows newer
	// templates, so its error is hidden.
	const [dismissedError, setDismissedError] = useState<unknown>();
	const shownError = error === dismissedError ? undefined : error;
	// Templates are shared by every editor of the project, so refresh them
	// on open and when the tab regains focus to detect changes made elsewhere.
	const projectsQuery = useQuery({
		...chatProjects(),
		refetchOnWindowFocus: true,
	});
	const isDraftUntouched =
		serializeValues(value) === serializeValues(initialValue);
	if (
		isDraftUntouched &&
		serializeTemplates(project.templates) !== serializeTemplates(baseTemplates)
	) {
		setDismissedError(error);
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
	const dropsDefaultTemplate =
		eligibleTemplates !== undefined &&
		value.defaultTemplateId !== "" &&
		defaultTemplate === undefined;
	const formValue: ChatProjectTemplatesValues = dropsDefaultTemplate
		? {
				...value,
				defaultTemplateId: "",
				defaultPresetId: "",
				defaultParameters: {},
			}
		: {
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
	// Without a fresh copy of the project, Save could overwrite a change
	// made elsewhere that the conflict check has not seen.
	const refreshFailed = projectsQuery.isError;
	const canSave =
		request !== undefined &&
		!isUnchanged &&
		!hasConflict &&
		!refreshFailed &&
		!projectsQuery.isFetching &&
		!isBusy;
	useEffect(() => {
		if (isBusy) {
			return;
		}
		const target =
			pendingFocusRef.current === "loadLatest" && hasConflict
				? loadLatestButtonRef.current
				: pendingFocusRef.current === "retry" && refreshFailed
					? retryButtonRef.current
					: pendingFocusRef.current === "save" && shownError
						? saveButtonRef.current
						: null;
		if (target) {
			pendingFocusRef.current = null;
			target.focus();
		}
	}, [isBusy, hasConflict, refreshFailed, shownError]);

	return (
		<form
			className="flex flex-col gap-4"
			onSubmit={async (event) => {
				event.preventDefault();
				if (!request || !canSave || isSubmittingRef.current) {
					return;
				}
				isSubmittingRef.current = true;
				// A change saved elsewhere since the last refresh would otherwise
				// be overwritten, so check first. A newer save shows the conflict
				// warning instead.
				// The draft is frozen until the check ends, so the request
				// still matches what the user sees.
				const checkId = onBeginCheck();
				const refreshed = await projectsQuery.refetch();
				isSubmittingRef.current = false;
				if (!onEndCheck(checkId)) {
					return;
				}
				if (refreshed.isError) {
					pendingFocusRef.current = "retry";
					return;
				}
				const latest = refreshed.data?.find(({ id }) => id === project.id);
				const serializedLatest = latest && serializeTemplates(latest.templates);
				if (serializedLatest === undefined) {
					return;
				}
				if (
					serializedLatest !== serializeTemplates(baseTemplates) &&
					serializedLatest !== serializeTemplates(request)
				) {
					pendingFocusRef.current = "loadLatest";
					return;
				}
				// A rejected save leaves the dialog open, so focus returns to Save.
				pendingFocusRef.current = "save";
				onSave(request);
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
				onChange={(next) => {
					setValue(next);
					if (shownError) {
						onResetError();
					}
				}}
				disabled={isBusy}
			/>
			{(droppedTemplateCount > 0 ||
				hasStalePreset ||
				staleParameterNames.length > 0) && (
				<Alert severity="warning">
					{droppedTemplateCount > 0 && (
						<AlertDescription className="block">
							{droppedTemplateCount === 1
								? "1 template is"
								: `${droppedTemplateCount} templates are`}{" "}
							no longer available to agents (deleted, deprecated, or no longer
							allowed for agents). Saving removes{" "}
							{droppedTemplateCount === 1 ? "it" : "them"} from the project
							{dropsDefaultTemplate &&
								", along with the default template and its default values"}
							.
						</AlertDescription>
					)}
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
							disabled={projectsQuery.isFetching}
							onClick={() => void projectsQuery.refetch()}
						>
							Retry
						</Button>
					}
				>
					<AlertDescription>
						Couldn't check for template changes made elsewhere, so saving is
						paused.
					</AlertDescription>
				</Alert>
			)}
			{hasConflict && (
				<ProjectEditConflictAlert
					loadLatestButtonRef={loadLatestButtonRef}
					disabled={isBusy}
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
			{Boolean(shownError) && !isApiValidationError(shownError) && (
				<ErrorAlert error={shownError} showDebugDetail={false} />
			)}
			<DialogFooter>
				<Button
					type="button"
					variant="outline"
					disabled={isBusy}
					onClick={onCancel}
				>
					Cancel
				</Button>
				<Button
					ref={saveButtonRef}
					type="submit"
					disabled={!canSave}
					aria-busy={isBusy || projectsQuery.isFetching}
				>
					<Spinner loading={isBusy || projectsQuery.isFetching} />
					Save
				</Button>
			</DialogFooter>
		</form>
	);
};
