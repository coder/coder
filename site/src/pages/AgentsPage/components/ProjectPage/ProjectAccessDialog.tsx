import { EllipsisVerticalIcon, UserPlusIcon } from "lucide-react";
import { useId, useRef, useState } from "react";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "#/api/errors";
import type { ChatProjectRole } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
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
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { RetryButton } from "#/components/RetryButton/RetryButton";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { isGroup } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";
import type {
	AccessChangeCallbacks,
	ProjectAccessEntry,
	ProjectAccessPrincipal,
} from "./projectAccess";

/** The empty role only removes access in update requests. */
const chatProjectRoleLabel: Record<Exclude<ChatProjectRole, "">, string> = {
	admin: "Admin",
	use: "Member",
};

type AssignableRole = Exclude<ChatProjectRole, "">;

const roleDescriptions: Record<AssignableRole, string> = {
	admin:
		"Can edit the project, its instructions and skills, and manage access.",
	use: "Can view the project and start chats in it.",
};

type ProjectAccessDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	/** `undefined` while the access list loads or after loading fails. */
	readonly entries: readonly ProjectAccessEntry[] | undefined;
	readonly error: unknown;
	readonly isRetrying: boolean;
	readonly onRetry: () => void;
	readonly onCloseAutoFocus: (event: Event) => void;
	/** Whether the viewer can add, change, and remove access. */
	readonly canManage: boolean;
	readonly organizationId: string;
	/** The viewer cannot change their own access. */
	readonly currentUserId: string;
	/** Grants access. Each change runs its callbacks once it saves or fails. */
	readonly onAdd: (
		principal: ProjectAccessPrincipal,
		role: AssignableRole,
		callbacks: AccessChangeCallbacks,
	) => void;
	readonly onChangeRole: ProjectAccessDialogProps["onAdd"];
	readonly onRemove: (
		principal: ProjectAccessPrincipal,
		callbacks: AccessChangeCallbacks,
	) => void;
	/** The user or group whose change is saving, including one being added. */
	readonly pendingPrincipalId: string | undefined;
	readonly mutationError: unknown;
};

export const ProjectAccessDialog: React.FC<ProjectAccessDialogProps> = ({
	open,
	onOpenChange,
	entries,
	error,
	isRetrying,
	onRetry,
	onCloseAutoFocus,
	canManage,
	organizationId,
	currentUserId,
	onAdd,
	onChangeRole,
	onRemove,
	pendingPrincipalId,
	mutationError,
}) => {
	const addTriggerRef = useRef<HTMLButtonElement>(null);
	// Announces each change's result, since saving moves no focus.
	const [savedStatus, setSavedStatus] = useState("");
	const status =
		pendingPrincipalId === undefined ? savedStatus : "Saving access changes…";

	const focusAddTrigger = () => addTriggerRef.current?.focus();

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				className="flex max-w-2xl flex-col gap-0 overflow-hidden p-0"
				onCloseAutoFocus={onCloseAutoFocus}
			>
				<DialogHeader className="px-6 pt-6">
					<DialogTitle>
						{canManage ? "Manage access" : "View access"}
					</DialogTitle>
				</DialogHeader>
				<div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-4">
					{canManage && entries && (
						<AddAccessForm
							triggerRef={addTriggerRef}
							organizationId={organizationId}
							exclude={[...entries, { id: currentUserId }]}
							pendingPrincipalId={pendingPrincipalId}
							onAdd={(principal, role, callbacks) => {
								setSavedStatus("");
								onAdd(principal, role, {
									...callbacks,
									onSuccess: () => {
										callbacks.onSuccess?.();
										setSavedStatus("Access added.");
									},
								});
							}}
							onSettled={focusAddTrigger}
						/>
					)}
					{Boolean(mutationError) && (
						<Alert severity="error" prominent>
							<AlertTitle>
								{getErrorMessage(mutationError, "Failed to change access.")}
							</AlertTitle>
							<AlertDescription>
								{/* The generic 403 detail is about viewing content, not sharing. */}
								{getErrorStatus(mutationError) === 403
									? "You don't have permission to change access to this project."
									: getErrorDetail(mutationError)}
							</AlertDescription>
						</Alert>
					)}
					{Boolean(error) && (
						<ErrorAlert
							error={error}
							showDebugDetail={false}
							actions={
								<RetryButton
									size="sm"
									variant="outline"
									isRetrying={isRetrying}
									onRetry={onRetry}
								/>
							}
						/>
					)}
					{(entries || !error) && (
						<Table
							aria-label="Project access"
							className="max-sm:block"
							// Grows the scroll area instead of scrolling inside it.
							wrapperClassName="shrink-0"
						>
							<TableHeader className="max-sm:sr-only">
								<TableRow>
									<TableHead className="w-full">User or group</TableHead>
									<TableHead>Role</TableHead>
									{canManage && (
										<TableHead>
											<span className="sr-only">Actions</span>
										</TableHead>
									)}
								</TableRow>
							</TableHeader>
							{/* Phones stack each row's role and actions under its name. */}
							<TableBody className="max-sm:block max-sm:rounded-md max-sm:border max-sm:[&>tr:last-child]:border-b-0 max-sm:[&>tr>td:first-child]:col-span-full max-sm:[&>tr>td:not(:first-child)]:pt-0 max-sm:[&>tr>td]:border-0! max-sm:[&>tr]:grid max-sm:[&>tr]:grid-cols-[minmax(0,1fr)_auto]">
								{entries ? (
									entries.map((entry) => (
										<AccessRow
											key={`${entry.kind}:${entry.id}`}
											entry={entry}
											canManage={canManage}
											isCurrentUser={
												entry.kind === "user" && entry.id === currentUserId
											}
											disabled={pendingPrincipalId !== undefined}
											onChangeRole={(principal, role) => {
												setSavedStatus("");
												onChangeRole(principal, role, {
													onSuccess: () =>
														setSavedStatus(
															`${entry.name} is now ${chatProjectRoleLabel[role]}.`,
														),
												});
											}}
											onRemove={(principal) => {
												setSavedStatus("");
												onRemove(principal, {
													onSuccess: () => {
														setSavedStatus(`${entry.name} was removed.`);
														focusAddTrigger();
													},
												});
											}}
										/>
									))
								) : (
									<TableLoader />
								)}
							</TableBody>
						</Table>
					)}
					<p role="status" className="sr-only">
						{status}
					</p>
					<DialogDescription className="font-normal">
						Chats in this project start private. Their creators can still share
						them individually.
					</DialogDescription>
				</div>
				<DialogFooter className="border-t px-6 py-4">
					<Button variant="outline" onClick={() => onOpenChange(false)}>
						Close
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

type AddAccessFormProps = {
	readonly triggerRef: React.Ref<HTMLButtonElement>;
	readonly organizationId: string;
	readonly exclude: { readonly id: string }[];
	readonly pendingPrincipalId: string | undefined;
	readonly onAdd: ProjectAccessDialogProps["onAdd"];
	readonly onSettled: () => void;
};

const AddAccessForm: React.FC<AddAccessFormProps> = ({
	triggerRef,
	organizationId,
	exclude,
	pendingPrincipalId,
	onAdd,
	onSettled,
}) => {
	const triggerId = useId();
	const [draft, setDraft] = useState<{
		selection: UserOrGroupAutocompleteValue;
		role: AssignableRole;
	}>({ selection: null, role: "use" });
	const { selection, role } = draft;
	// The selection is never listed yet, so a pending change for it is this add.
	const isAdding = selection !== null && selection.id === pendingPrincipalId;
	const isChangePending = pendingPrincipalId !== undefined;

	return (
		<form
			className="flex flex-col gap-2 sm:flex-row sm:items-center"
			onSubmit={(event) => {
				event.preventDefault();
				if (!selection || isChangePending) {
					return;
				}
				onAdd(
					{ kind: isGroup(selection) ? "group" : "user", id: selection.id },
					role,
					{
						// Keep a selection the user changed while this add saved.
						onSuccess: () =>
							setDraft((current) =>
								current.selection?.id === selection.id
									? { selection: null, role: "use" }
									: current,
							),
						onSettled,
					},
				);
			}}
		>
			<div className="min-w-0 flex-1">
				<span id={`${triggerId}-label`} className="sr-only">
					User or group
				</span>
				<UserOrGroupAutocomplete
					id={triggerId}
					triggerRef={triggerRef}
					// The name is the label, then the selection the trigger shows.
					triggerAriaLabelledBy={`${triggerId}-label ${triggerId}`}
					value={selection}
					onChange={(selection) =>
						setDraft((current) => ({ ...current, selection }))
					}
					organizationId={organizationId}
					exclude={exclude}
					className="w-full"
				/>
			</div>
			<ProjectRoleSelect
				value={role}
				label="Role"
				disabled={isAdding}
				onValueChange={(role) => setDraft((current) => ({ ...current, role }))}
			/>
			<Button type="submit" disabled={!selection || isChangePending}>
				<Spinner loading={isAdding}>
					<UserPlusIcon />
				</Spinner>
				Add
			</Button>
		</form>
	);
};

type ProjectRoleSelectProps = {
	/** `""` shows the placeholder. */
	readonly value: ChatProjectRole;
	readonly label: string;
	readonly disabled: boolean;
	readonly onValueChange: (role: AssignableRole) => void;
	readonly ref?: React.Ref<HTMLButtonElement>;
};

const ProjectRoleSelect: React.FC<ProjectRoleSelectProps> = ({
	value,
	label,
	disabled,
	onValueChange,
	ref,
}) => {
	const [open, setOpen] = useState(false);

	// Disabled with aria-disabled, so the trigger keeps focus while saving.
	return (
		<Select
			value={value}
			open={open}
			onOpenChange={(next) => setOpen(next && !disabled)}
			onValueChange={(role) => {
				if (!disabled && (role === "admin" || role === "use")) {
					onValueChange(role);
				}
			}}
		>
			<SelectTrigger
				ref={ref}
				className="w-full sm:w-32 aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
				aria-label={label}
				aria-disabled={disabled || undefined}
			>
				<SelectValue placeholder="Role">
					{value === "" ? undefined : chatProjectRoleLabel[value]}
				</SelectValue>
			</SelectTrigger>
			<SelectContent align="end">
				{(["use", "admin"] as const).map((role) => (
					<SelectItem
						key={role}
						value={role}
						className="w-64 flex-col items-start py-2"
					>
						<div className="font-medium text-content-primary">
							{chatProjectRoleLabel[role]}
						</div>
						<div className="mt-0.5 text-xs leading-snug text-content-secondary">
							{roleDescriptions[role]}
						</div>
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
};

type AccessRowProps = {
	readonly entry: ProjectAccessEntry;
	readonly canManage: boolean;
	readonly isCurrentUser: boolean;
	readonly disabled: boolean;
	readonly onChangeRole: (
		principal: ProjectAccessPrincipal,
		role: AssignableRole,
	) => void;
	readonly onRemove: (principal: ProjectAccessPrincipal) => void;
};

const AccessRow: React.FC<AccessRowProps> = ({
	entry,
	canManage,
	isCurrentUser,
	disabled,
	onChangeRole,
	onRemove,
}) => {
	const editableEntry =
		canManage && entry.kind !== "owner" && !isCurrentUser ? entry : undefined;
	const [isMenuOpen, setIsMenuOpen] = useState(false);

	return (
		<TableRow>
			{/* With the header's w-full, max-w-0 lets long names truncate. */}
			<TableCell className="min-w-0 sm:max-w-0">
				<AvatarData
					truncate
					title={entry.name}
					subtitle={entry.subtitle}
					src={entry.avatarUrl}
				/>
			</TableCell>
			<TableCell>
				{editableEntry ? (
					<ProjectRoleSelect
						value={editableEntry.role}
						label={`Role for ${editableEntry.name}`}
						disabled={disabled}
						onValueChange={(role) => onChangeRole(editableEntry, role)}
					/>
				) : entry.kind === "owner" ? (
					<Badge variant="info">Owner</Badge>
				) : (
					<Badge>
						{entry.role === "" ? "Unknown" : chatProjectRoleLabel[entry.role]}
					</Badge>
				)}
			</TableCell>
			{canManage && (
				<TableCell>
					{editableEntry && (
						<DropdownMenu
							open={isMenuOpen}
							onOpenChange={(next) => setIsMenuOpen(next && !disabled)}
						>
							<DropdownMenuTrigger asChild>
								<Button
									size="icon-lg"
									variant="subtle"
									aria-label={`Open menu for ${editableEntry.name}`}
									// Keeps focus on the button while a change saves.
									aria-disabled={disabled || undefined}
								>
									<EllipsisVerticalIcon aria-hidden="true" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onClick={() => onRemove(editableEntry)}
								>
									Remove
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					)}
				</TableCell>
			)}
		</TableRow>
	);
};
