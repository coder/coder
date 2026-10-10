import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import type { SkillOwner } from "#/api/queries/skills";
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
import {
	SKILLS_MAX_PER_OWNER,
	type SkillAccess,
	type SkillFormValues,
	type SkillsCopy,
} from "../utils/skills";
import { RetryButton } from "./RetryButton";
import { SectionHeader, type SectionHeaderLevel } from "./SectionHeader";
import type { SkillErrorDisplay } from "./SkillEditor";
import { SkillEditor } from "./SkillEditor";
import { SkillEnabledSwitch } from "./SkillEnabledSwitch";
import { useDialogFocusReturn } from "./useDialogFocusReturn";

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
	/** Opens the editor, read-only without update access. */
	onEdit: (name: string) => void;
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

type DialogProps<State> = {
	state: State;
	onCloseAutoFocus: (event: Event) => void;
};

const EditSkillDialog: React.FC<
	DialogProps<Extract<SkillEditorState, { mode: "edit" }>> & {
		copy: SkillsCopy;
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
							The skill could not be loaded for{" "}
							{state.readOnly ? "viewing" : "editing"}.
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
						<RetryButton
							isRetrying={state.isRetrying}
							onRetry={state.onRetry}
						/>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		);
	}

	return (
		<SkillEditor
			open
			mode="edit"
			readOnly={state.readOnly}
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

const DeleteSkillDialog: React.FC<DialogProps<SkillDeleteState>> = ({
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
	onDelete,
	onDownload,
	onManagePermissions,
	onExportAll,
	downloadingSkillName,
	isExportingAll,
	editorState,
	deleteState,
}) => {
	const {
		setPrimaryFallback,
		setSecondaryFallback,
		rememberTrigger,
		restoreFocus,
	} = useDialogFocusReturn();
	const pluralNoun = `${copy.noun.toLocaleLowerCase("en-US")}s`;
	const isAtLimit = skills.length >= SKILLS_MAX_PER_OWNER;
	const addSkillAction = (ref?: React.Ref<HTMLButtonElement>) =>
		access.create && (
			<Button
				ref={ref}
				variant="outline"
				onClick={(event) => {
					rememberTrigger(event);
					onCreate();
				}}
				disabled={isLoading || isAtLimit}
			>
				<PlusIcon />
				Add skill
			</Button>
		);
	const headerActions = (
		<div className="flex items-center gap-2">
			<Button
				ref={setSecondaryFallback}
				variant="outline"
				onClick={onExportAll}
				disabled={isLoading || isExportingAll || skills.length === 0}
			>
				{isExportingAll && <Spinner className="size-4" loading />}
				Export all
			</Button>
			{addSkillAction(setPrimaryFallback)}
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
							cta={addSkillAction()}
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
												onPointerDown={rememberTrigger}
												onKeyDown={rememberTrigger}
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
														onManagePermissions(skill, restoreFocus)
													}
												>
													Manage permissions
												</DropdownMenuItem>
											)}
											<DropdownMenuItem onClick={() => onEdit(skill.name)}>
												{access.update ? "Edit" : "View"}
											</DropdownMenuItem>
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
					onCloseAutoFocus={restoreFocus}
				/>
			)}
			{editorState?.mode === "edit" && (
				<EditSkillDialog
					copy={copy}
					state={editorState}
					onCloseAutoFocus={restoreFocus}
				/>
			)}
			{deleteState && (
				<DeleteSkillDialog
					state={deleteState}
					onCloseAutoFocus={restoreFocus}
				/>
			)}
		</div>
	);
};
