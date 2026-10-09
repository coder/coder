import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { useRef } from "react";
import type { SkillMetadata } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Loader } from "#/components/Loader/Loader";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatDate } from "#/utils/time";
import type { SkillFormValues } from "../utils/skills";
import { SectionHeader } from "./SectionHeader";
import type { SkillErrorDisplay } from "./SkillEditor";
import { SkillEditor } from "./SkillEditor";
import { TextPreviewDialog } from "./TextPreviewDialog";

export type SkillsCopy = {
	/** Singular noun in sentence case, for example "Personal skill". */
	noun: string;
	title: string;
	description: string;
	emptyDescription: string;
	editorDescription: string;
	archiveName: string;
};

export type SkillEditorState =
	| {
			mode: "create";
			initialValues: SkillFormValues;
			existingNames: readonly string[];
			submitError?: SkillErrorDisplay;
			isSubmitting: boolean;
			onSubmit: (values: SkillFormValues, content: string) => void;
			onClose: () => void;
	  }
	| {
			mode: "edit";
			initialValues?: SkillFormValues;
			existingNames: readonly string[];
			loadError?: unknown;
			isLoading: boolean;
			isRetrying: boolean;
			submitError?: SkillErrorDisplay;
			isSubmitting: boolean;
			onRetry: () => void;
			onSubmit: (values: SkillFormValues, content: string) => void;
			onClose: () => void;
	  };

export type SkillViewState = {
	name: string;
	content?: string;
	loadError?: unknown;
	isLoading: boolean;
	isRetrying: boolean;
	onRetry: () => void;
	onClose: () => void;
};

export type SkillDeleteState = {
	skill: SkillMetadata;
	error?: SkillErrorDisplay;
	isDeleting: boolean;
	onConfirm: () => void;
	onClose: () => void;
};

export type SkillsTableViewProps = {
	skills: readonly SkillMetadata[];
	copy: SkillsCopy;
	limit: number;
	canEdit: boolean;
	toolbar?: React.ReactNode;
	error: unknown;
	isLoading: boolean;
	isRetrying: boolean;
	onRetry: () => void;
	onCreate: () => void;
	onEdit: (name: string) => void;
	onView: (name: string) => void;
	onDelete: (skill: SkillMetadata) => void;
	onDownload: (skill: SkillMetadata) => void;
	onExportAll: () => void;
	onToggleEnabled: (skill: SkillMetadata, enabled: boolean) => void;
	downloadingSkillName?: string;
	togglingSkill?: { name: string; enabled: boolean };
	isExportingAll: boolean;
	editorState?: SkillEditorState;
	viewState?: SkillViewState;
	deleteState?: SkillDeleteState;
};

const formatUpdatedAt = (value: string) => {
	const date = new Date(value);
	if (!Number.isFinite(date.getTime())) {
		return "Unknown";
	}
	return formatDate(date, {
		locale: "en-US",
		month: "short",
		day: "numeric",
		year: "numeric",
		hour: "numeric",
		second: undefined,
		minute: "2-digit",
	});
};

type SkillLoadDialogProps = {
	noun: string;
	purpose: "editing" | "viewing";
	isLoading: boolean;
	loadError?: unknown;
	isRetrying: boolean;
	onRetry: () => void;
	onClose: () => void;
	onCloseAutoFocus: (event: Event) => void;
};

const SkillLoadDialog: React.FC<SkillLoadDialogProps> = ({
	noun,
	purpose,
	isLoading,
	loadError,
	isRetrying,
	onRetry,
	onClose,
	onCloseAutoFocus,
}) => {
	const lowerNoun = noun.toLocaleLowerCase("en-US");
	const handleOpenChange = (open: boolean) => {
		if (!open) {
			onClose();
		}
	};

	if (isLoading) {
		return (
			<Dialog open onOpenChange={handleOpenChange}>
				<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
					<DialogHeader>
						<DialogTitle>Loading {lowerNoun}</DialogTitle>
						<DialogDescription>
							Fetching the latest SKILL.md content.
						</DialogDescription>
					</DialogHeader>
					<Loader />
				</DialogContent>
			</Dialog>
		);
	}

	return (
		<Dialog open onOpenChange={handleOpenChange}>
			<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
				<DialogHeader>
					<DialogTitle>Unable to load {lowerNoun}</DialogTitle>
					<DialogDescription>
						The skill could not be loaded for {purpose}.
					</DialogDescription>
				</DialogHeader>
				{loadError ? (
					<ErrorAlert error={loadError} showDebugDetail={false} />
				) : (
					<Alert severity="error">
						<AlertDescription>
							The saved content could not be parsed as SKILL.md.
						</AlertDescription>
					</Alert>
				)}
				<DialogFooter>
					<Button variant="outline" onClick={onClose}>
						Close
					</Button>
					<Button onClick={onRetry} disabled={isRetrying}>
						{isRetrying && <Spinner className="size-4" loading />}
						Retry
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

type EditSkillDialogProps = {
	copy: SkillsCopy;
	state: Extract<SkillEditorState, { mode: "edit" }>;
	onCloseAutoFocus: (event: Event) => void;
};

const EditSkillDialog: React.FC<EditSkillDialogProps> = ({
	copy,
	state,
	onCloseAutoFocus,
}) => {
	if (state.isLoading || state.loadError || !state.initialValues) {
		return (
			<SkillLoadDialog
				noun={copy.noun}
				purpose="editing"
				isLoading={state.isLoading}
				loadError={state.loadError}
				isRetrying={state.isRetrying}
				onRetry={state.onRetry}
				onClose={state.onClose}
				onCloseAutoFocus={onCloseAutoFocus}
			/>
		);
	}

	return (
		<SkillEditor
			open
			mode="edit"
			noun={copy.noun}
			description={copy.editorDescription}
			initialValues={state.initialValues}
			existingNames={state.existingNames}
			submitError={state.submitError}
			isSubmitting={state.isSubmitting}
			onOpenChange={(open) => {
				if (!open) {
					state.onClose();
				}
			}}
			onCloseAutoFocus={onCloseAutoFocus}
			onSubmit={state.onSubmit}
		/>
	);
};

type ViewSkillDialogProps = {
	noun: string;
	state: SkillViewState;
	onCloseAutoFocus: (event: Event) => void;
};

const ViewSkillDialog: React.FC<ViewSkillDialogProps> = ({
	noun,
	state,
	onCloseAutoFocus,
}) => {
	if (state.content === undefined) {
		return (
			<SkillLoadDialog
				noun={noun}
				purpose="viewing"
				isLoading={state.isLoading}
				loadError={state.loadError}
				isRetrying={state.isRetrying}
				onRetry={state.onRetry}
				onClose={state.onClose}
				onCloseAutoFocus={onCloseAutoFocus}
			/>
		);
	}

	return (
		<TextPreviewDialog
			content={state.content}
			fileName={state.name}
			onClose={state.onClose}
			onCloseAutoFocus={onCloseAutoFocus}
			showCloseButton
		/>
	);
};

type DeleteSkillDialogProps = {
	state: SkillDeleteState;
	onCloseAutoFocus: (event: Event) => void;
};

const DeleteSkillDialog: React.FC<DeleteSkillDialogProps> = ({
	state,
	onCloseAutoFocus,
}) => {
	return (
		<ConfirmDialog
			type="delete"
			open
			onClose={state.onClose}
			onCloseAutoFocus={onCloseAutoFocus}
			title="Delete skill"
			confirmText="Delete skill"
			description={
				<>
					<p className="m-0">
						Delete {state.skill.name}? Agents will no longer be able to use this
						skill. This action cannot be undone.
					</p>
					{state.error && (
						<Alert severity="error" className="mt-3">
							<AlertDescription>
								{state.error.message}
								{state.error.detail ? ` ${state.error.detail}` : ""}
							</AlertDescription>
						</Alert>
					)}
				</>
			}
			onConfirm={state.onConfirm}
			confirmLoading={state.isDeleting}
		/>
	);
};

type SkillEnabledSwitchProps = {
	skill: SkillMetadata;
	noun: string;
	checked: boolean;
	isBlocked: boolean;
	readOnlyReason?: string;
	onToggleEnabled: (skill: SkillMetadata, enabled: boolean) => void;
};

const SkillEnabledSwitch: React.FC<SkillEnabledSwitchProps> = ({
	skill,
	noun,
	checked,
	isBlocked,
	readOnlyReason,
	onToggleEnabled,
}) => {
	// aria-disabled instead of disabled keeps a read-only switch focusable, so
	// keyboard users can reach the tooltip that explains why.
	const toggle = (
		<Switch
			checked={checked}
			aria-label={
				readOnlyReason ? `${noun} ${skill.name}` : `Enable ${skill.name}`
			}
			aria-disabled={isBlocked || undefined}
			onCheckedChange={(enabled) => {
				if (!isBlocked) {
					onToggleEnabled(skill, enabled);
				}
			}}
		/>
	);
	if (!readOnlyReason) {
		return toggle;
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>{toggle}</TooltipTrigger>
			<TooltipContent side="bottom">{readOnlyReason}</TooltipContent>
		</Tooltip>
	);
};

type AddSkillButtonProps = {
	ref?: React.Ref<HTMLButtonElement>;
	disabled: boolean;
	onClick: React.MouseEventHandler<HTMLButtonElement>;
};

const canFocus = (
	button: HTMLButtonElement | null,
): button is HTMLButtonElement =>
	Boolean(button?.isConnected && !button.disabled);

const AddSkillButton: React.FC<AddSkillButtonProps> = ({
	ref,
	disabled,
	onClick,
}) => (
	<Button ref={ref} variant="outline" onClick={onClick} disabled={disabled}>
		<PlusIcon />
		Add skill
	</Button>
);

export const SkillsTableView: React.FC<SkillsTableViewProps> = ({
	skills,
	copy,
	limit,
	canEdit,
	toolbar,
	error,
	isLoading,
	isRetrying,
	onRetry,
	onCreate,
	onEdit,
	onView,
	onDelete,
	onDownload,
	onExportAll,
	onToggleEnabled,
	downloadingSkillName,
	togglingSkill,
	isExportingAll,
	editorState,
	viewState,
	deleteState,
}) => {
	const dialogTriggerRef = useRef<HTMLButtonElement | null>(null);
	const addSkillButtonRef = useRef<HTMLButtonElement | null>(null);
	const exportAllButtonRef = useRef<HTMLButtonElement | null>(null);
	const rememberDialogTrigger = (
		event: React.SyntheticEvent<HTMLButtonElement>,
	) => {
		dialogTriggerRef.current = event.currentTarget;
	};
	// These dialogs open without a Radix DialogTrigger, so Radix would
	// otherwise return focus to the document body on close. An unmounted or
	// disabled opener falls back to the header actions.
	const restoreDialogFocus = (event: Event) => {
		// An array find here would stop the React Compiler memoizing this closure.
		let target = dialogTriggerRef.current;
		if (!canFocus(target)) {
			target = addSkillButtonRef.current;
		}
		if (!canFocus(target)) {
			target = exportAllButtonRef.current;
		}
		if (canFocus(target)) {
			event.preventDefault();
			target.focus();
		}
	};
	const openCreateDialog = (event: React.MouseEvent<HTMLButtonElement>) => {
		rememberDialogTrigger(event);
		onCreate();
	};
	const pluralNoun = `${copy.noun.toLocaleLowerCase("en-US")}s`;
	const isAtLimit = skills.length >= limit;
	const readOnlyReason = canEdit
		? undefined
		: `You do not have permission to change ${pluralNoun}.`;
	const addSkillDisabled = isLoading || isAtLimit;
	const headerActions = (
		<div className="flex items-center gap-2">
			<Button
				ref={exportAllButtonRef}
				variant="outline"
				onClick={onExportAll}
				disabled={isLoading || isExportingAll || skills.length === 0}
			>
				{isExportingAll && <Spinner className="size-4" loading />}
				Export all
			</Button>
			{canEdit && (
				<AddSkillButton
					ref={addSkillButtonRef}
					disabled={addSkillDisabled}
					onClick={openCreateDialog}
				/>
			)}
		</div>
	);

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label={copy.title}
				description={copy.description}
				action={headerActions}
			/>

			{toolbar}

			{canEdit && isAtLimit && (
				<Alert severity="warning">
					<AlertDescription>
						You have reached the limit of {limit} {pluralNoun}. Delete a skill
						before creating another one.
					</AlertDescription>
				</Alert>
			)}

			{Boolean(error) && <ErrorAlert error={error} />}

			<Table aria-label={copy.title}>
				<TableHeader>
					<TableRow>
						<TableHead className="whitespace-nowrap">Name</TableHead>
						<TableHead className="w-full">Description</TableHead>
						<TableHead className="whitespace-nowrap">Enabled</TableHead>
						<TableHead className="hidden whitespace-nowrap sm:table-cell">
							Updated
						</TableHead>
						<TableHead className="w-14">
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody size="lg">
					{isLoading ? (
						<TableLoaderSkeleton>
							<TableRowSkeleton aria-label={`Loading ${pluralNoun}`}>
								<TableCell>
									<Skeleton variant="text" className="w-32" />
								</TableCell>
								<TableCell className="w-full max-w-0">
									<Skeleton variant="text" />
								</TableCell>
								<TableCell>
									<Skeleton className="h-5 w-9" />
								</TableCell>
								<TableCell className="hidden sm:table-cell">
									<Skeleton variant="text" className="w-44" />
								</TableCell>
								<TableCell>
									<Skeleton className="size-8" />
								</TableCell>
							</TableRowSkeleton>
						</TableLoaderSkeleton>
					) : skills.length === 0 && error ? (
						<TableEmpty
							message={`Failed to load ${pluralNoun}`}
							cta={
								<Button
									variant="outline"
									onClick={onRetry}
									disabled={isRetrying}
								>
									{isRetrying && <Spinner className="size-4" loading />}
									Retry
								</Button>
							}
						/>
					) : skills.length === 0 ? (
						<TableEmpty
							message={`No ${pluralNoun} yet`}
							description={canEdit ? copy.emptyDescription : undefined}
							cta={
								canEdit && (
									<AddSkillButton
										disabled={addSkillDisabled}
										onClick={openCreateDialog}
									/>
								)
							}
						/>
					) : (
						skills.map((skill) => (
							<TableRow key={skill.id}>
								<TableCell className="max-w-48 truncate" title={skill.name}>
									{skill.name}
								</TableCell>
								<TableCell
									className="w-full max-w-0 truncate"
									title={skill.description || undefined}
								>
									{skill.description || (
										<span className="text-content-disabled">
											No description
										</span>
									)}
								</TableCell>
								<TableCell>
									<SkillEnabledSwitch
										skill={skill}
										noun={copy.noun}
										checked={
											togglingSkill?.name === skill.name
												? togglingSkill.enabled
												: skill.enabled
										}
										isBlocked={!canEdit || togglingSkill?.name === skill.name}
										readOnlyReason={readOnlyReason}
										onToggleEnabled={onToggleEnabled}
									/>
								</TableCell>
								<TableCell className="hidden whitespace-nowrap sm:table-cell">
									{formatUpdatedAt(skill.updated_at)}
								</TableCell>
								<TableCell className="text-right">
									<DropdownMenu>
										<DropdownMenuTrigger asChild>
											<Button
												size="icon"
												variant="subtle"
												aria-label="Open menu"
												onPointerDown={rememberDialogTrigger}
												onKeyDown={rememberDialogTrigger}
											>
												{downloadingSkillName === skill.name ? (
													<Spinner className="size-4" loading />
												) : (
													<EllipsisVerticalIcon aria-hidden="true" />
												)}
											</Button>
										</DropdownMenuTrigger>
										<DropdownMenuContent align="end">
											<DropdownMenuItem
												onClick={() => onDownload(skill)}
												disabled={downloadingSkillName === skill.name}
											>
												Download
											</DropdownMenuItem>
											{canEdit ? (
												<>
													<DropdownMenuItem onClick={() => onEdit(skill.name)}>
														Edit
													</DropdownMenuItem>
													<DropdownMenuSeparator />
													<DropdownMenuItem
														className="text-content-destructive focus:text-content-destructive"
														onClick={() => onDelete(skill)}
													>
														Delete&hellip;
													</DropdownMenuItem>
												</>
											) : (
												<DropdownMenuItem onClick={() => onView(skill.name)}>
													View
												</DropdownMenuItem>
											)}
										</DropdownMenuContent>
									</DropdownMenu>
								</TableCell>
							</TableRow>
						))
					)}
				</TableBody>
			</Table>

			{editorState?.mode === "create" && (
				<SkillEditor
					open
					mode="create"
					noun={copy.noun}
					description={copy.editorDescription}
					initialValues={editorState.initialValues}
					existingNames={editorState.existingNames}
					submitError={editorState.submitError}
					isSubmitting={editorState.isSubmitting}
					onOpenChange={(open) => {
						if (!open) {
							editorState.onClose();
						}
					}}
					onSubmit={editorState.onSubmit}
					onCloseAutoFocus={restoreDialogFocus}
				/>
			)}
			{editorState?.mode === "edit" && (
				<EditSkillDialog
					copy={copy}
					state={editorState}
					onCloseAutoFocus={restoreDialogFocus}
				/>
			)}
			{viewState && (
				<ViewSkillDialog
					noun={copy.noun}
					state={viewState}
					onCloseAutoFocus={restoreDialogFocus}
				/>
			)}
			{deleteState && (
				<DeleteSkillDialog
					state={deleteState}
					onCloseAutoFocus={restoreDialogFocus}
				/>
			)}
		</div>
	);
};
