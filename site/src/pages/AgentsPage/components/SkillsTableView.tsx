import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { useRef } from "react";
import type { SkillOwner } from "#/api/queries/skills";
import type { SkillMetadata } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
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
import { SectionHeader, type SectionHeaderLevel } from "./SectionHeader";
import {
	DeleteSkillDialog,
	EditSkillDialog,
	RetryButton,
} from "./SkillDialogs";
import type { SkillErrorDisplay } from "./SkillEditor";
import { SkillEditor } from "./SkillEditor";
import { SkillEnabledSwitch } from "./SkillEnabledSwitch";

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
			readOnly?: boolean;
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

export const readOnlySkillAccess: SkillAccess = {
	create: false,
	update: false,
	delete: false,
};

export type SkillsTableViewProps = {
	owner: SkillOwner;
	skills: readonly SkillMetadata[];
	copy: SkillsCopy;
	access: SkillAccess;
	headerLevel?: SectionHeaderLevel;
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
	onManagePermissions?: (
		skill: SkillMetadata,
		onCloseAutoFocus: (event: Event) => void,
	) => void;
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
	owner,
	skills,
	copy,
	access,
	headerLevel,
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
	onManagePermissions,
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
				level={headerLevel}
			/>

			{toolbar}

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
								<RetryButton
									variant="outline"
									isRetrying={isRetrying}
									onRetry={onRetry}
								/>
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
								<TableCell>
									<SkillEnabledSwitch
										owner={owner}
										skill={skill}
										disabled={!access.update}
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
											{onManagePermissions && (
												<DropdownMenuItem
													onClick={() =>
														onManagePermissions(skill, restoreDialogFocus)
													}
												>
													Manage permissions
												</DropdownMenuItem>
											)}
											{access.update ? (
												<DropdownMenuItem onClick={() => onEdit(skill.name)}>
													Edit
												</DropdownMenuItem>
											) : (
												<DropdownMenuItem onClick={() => onView(skill.name)}>
													View
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
