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
import { formatDate } from "#/utils/time";
import { SKILLS_MAX_PER_OWNER, type SkillFormValues } from "../utils/skills";
import { SectionHeader } from "./SectionHeader";
import type { SkillErrorDisplay } from "./SkillEditor";
import { SkillEditor } from "./SkillEditor";

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

export type SkillDeleteState = {
	skill: SkillMetadata;
	error?: SkillErrorDisplay;
	isDeleting: boolean;
	onConfirm: () => void;
	onClose: () => void;
};

export type SkillAccess = {
	create: boolean;
	update: boolean;
	delete: boolean;
};

export const fullSkillAccess: SkillAccess = {
	create: true,
	update: true,
	delete: true,
};

export type SkillsTableViewProps = {
	skills: readonly SkillMetadata[];
	copy: SkillsCopy;
	access: SkillAccess;
	error: unknown;
	isLoading: boolean;
	isRetrying: boolean;
	onRetry: () => void;
	onCreate: () => void;
	onEdit: (name: string) => void;
	onDelete: (skill: SkillMetadata) => void;
	onDownload: (skill: SkillMetadata) => void;
	onExportAll: () => void;
	downloadingSkillName?: string;
	isExportingAll: boolean;
	editorState?: SkillEditorState;
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

type DialogFocusProps = {
	onCloseAutoFocus: (event: Event) => void;
};

const EditSkillDialog: React.FC<
	DialogFocusProps & {
		copy: SkillsCopy;
		state: Extract<SkillEditorState, { mode: "edit" }>;
	}
> = ({ copy, state, onCloseAutoFocus }) => {
	const lowerNoun = copy.noun.toLocaleLowerCase("en-US");
	const handleOpenChange = (open: boolean) => {
		if (!open) {
			state.onClose();
		}
	};

	if (state.isLoading) {
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

	if (state.loadError || !state.initialValues) {
		return (
			<Dialog open onOpenChange={handleOpenChange}>
				<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
					<DialogHeader>
						<DialogTitle>Unable to load {lowerNoun}</DialogTitle>
						<DialogDescription>
							The skill could not be loaded for editing.
						</DialogDescription>
					</DialogHeader>
					{state.loadError ? (
						<ErrorAlert error={state.loadError} showDebugDetail={false} />
					) : (
						<Alert severity="error">
							<AlertDescription>
								The saved content could not be parsed as SKILL.md.
							</AlertDescription>
						</Alert>
					)}
					<DialogFooter>
						<Button variant="outline" onClick={state.onClose}>
							Close
						</Button>
						<Button onClick={state.onRetry} disabled={state.isRetrying}>
							{state.isRetrying && <Spinner className="size-4" loading />}
							Retry
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
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
			onOpenChange={handleOpenChange}
			onCloseAutoFocus={onCloseAutoFocus}
			onSubmit={state.onSubmit}
		/>
	);
};

const DeleteSkillDialog: React.FC<
	DialogFocusProps & { state: SkillDeleteState }
> = ({ state, onCloseAutoFocus }) => {
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

type AddSkillButtonProps = {
	ref?: React.Ref<HTMLButtonElement>;
	disabled: boolean;
	onClick: React.MouseEventHandler<HTMLButtonElement>;
};

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

const canFocus = (
	button: HTMLButtonElement | null,
): button is HTMLButtonElement =>
	Boolean(button?.isConnected && !button.disabled);

export const SkillsTableView: React.FC<SkillsTableViewProps> = ({
	skills,
	copy,
	access,
	error,
	isLoading,
	isRetrying,
	onRetry,
	onCreate,
	onEdit,
	onDelete,
	onDownload,
	onExportAll,
	downloadingSkillName,
	isExportingAll,
	editorState,
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
	const isAtLimit = skills.length >= SKILLS_MAX_PER_OWNER;
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
			{access.create && (
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

			{access.create && isAtLimit && (
				<Alert severity="warning">
					<AlertDescription>
						You have reached the limit of {SKILLS_MAX_PER_OWNER} {pluralNoun}.
						Delete a skill before creating another one.
					</AlertDescription>
				</Alert>
			)}

			{Boolean(error) && <ErrorAlert error={error} />}

			<Table aria-label={copy.title}>
				<TableHeader>
					<TableRow>
						<TableHead className="whitespace-nowrap">Name</TableHead>
						<TableHead className="w-full">Description</TableHead>
						<TableHead className="whitespace-nowrap">Updated</TableHead>
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
							description={access.create ? copy.emptyDescription : undefined}
							cta={
								access.create && (
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
								<TableCell className="whitespace-nowrap">
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
											{access.update && (
												<DropdownMenuItem onClick={() => onEdit(skill.name)}>
													Edit
												</DropdownMenuItem>
											)}
											{access.delete && (
												<>
													<DropdownMenuSeparator />
													<DropdownMenuItem
														className="text-content-destructive focus:text-content-destructive"
														onClick={() => onDelete(skill)}
													>
														Delete&hellip;
													</DropdownMenuItem>
												</>
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
			{deleteState && (
				<DeleteSkillDialog
					state={deleteState}
					onCloseAutoFocus={restoreDialogFocus}
				/>
			)}
		</div>
	);
};
