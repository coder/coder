import { EllipsisVerticalIcon, UserPlusIcon } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
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
import { RetryButton } from "../RetryButton";
import {
	type AccessChangeCallbacks,
	type AssignableRole,
	chatProjectRoleLabel,
	type ProjectAccessEntry,
	type ProjectAccessPrincipal,
} from "./projectAccess";

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

/** Lists who can use a project, and lets owners and admins change it; onCloseAutoFocus restores focus. */
export const ProjectAccessDialog: React.FC<ProjectAccessDialogProps> = (
	props,
) => <ProjectAccessDialogContent {...props} addTriggerId={useId()} />;

type ProjectAccessDialogContentProps = ProjectAccessDialogProps & {
	readonly addTriggerId: string;
};

const ProjectAccessDialogContent: React.FC<ProjectAccessDialogContentProps> = ({
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
	addTriggerId,
}) => (
	<Dialog open={open} onOpenChange={onOpenChange}>
		<DialogContent
			className="flex max-w-2xl flex-col gap-0 overflow-hidden p-0"
			onCloseAutoFocus={onCloseAutoFocus}
		>
			<DialogHeader className="px-6 pt-6">
				<DialogTitle>{canManage ? "Manage access" : "View access"}</DialogTitle>
			</DialogHeader>
			<div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-4">
				{canManage && entries && (
					<AddAccessForm
						triggerId={addTriggerId}
						organizationId={organizationId}
						exclude={[...entries, { id: currentUserId }]}
						pendingPrincipalId={pendingPrincipalId}
						onAdd={onAdd}
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
						<TableHeader className="max-sm:hidden">
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
						<TableBody className="max-sm:block max-sm:rounded-md max-sm:border max-sm:border-border max-sm:border-solid max-sm:[&>tr:last-child]:border-b-0 max-sm:[&>tr>td:first-child]:col-span-full max-sm:[&>tr>td:not(:first-child)]:pt-0 max-sm:[&>tr>td]:border-0! max-sm:[&>tr]:grid max-sm:[&>tr]:grid-cols-[minmax(0,1fr)_auto]">
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
										onChangeRole={onChangeRole}
										onRemove={onRemove}
										onRemoved={() => focusById(addTriggerId)}
									/>
								))
							) : (
								<TableLoader />
							)}
						</TableBody>
					</Table>
				)}
				<DialogDescription className="font-normal">
					Chats in this project start private. Their creators can still share
					them individually.
				</DialogDescription>
			</div>
			<DialogFooter className="border-t border-border px-6 py-4">
				<Button variant="outline" onClick={() => onOpenChange(false)}>
					Close
				</Button>
			</DialogFooter>
		</DialogContent>
	</Dialog>
);

// Returns focus to the add row after saving a change drops it.
const focusById = (id: string) => document.getElementById(id)?.focus();

type AddAccessFormProps = {
	readonly triggerId: string;
	readonly organizationId: string;
	readonly exclude: { readonly id: string }[];
	readonly pendingPrincipalId: string | undefined;
	readonly onAdd: ProjectAccessDialogProps["onAdd"];
};

const AddAccessForm: React.FC<AddAccessFormProps> = ({
	triggerId,
	organizationId,
	exclude,
	pendingPrincipalId,
	onAdd,
}) => {
	const [draft, setDraft] = useState<{
		selection: UserOrGroupAutocompleteValue;
		role: AssignableRole | "";
	}>({ selection: null, role: "" });
	const { selection, role } = draft;
	// The selection is never listed yet, so a pending change for it is this add.
	const isAdding = selection !== null && selection.id === pendingPrincipalId;
	const isChangePending = pendingPrincipalId !== undefined;

	return (
		<form
			className="flex flex-col gap-2 sm:flex-row sm:items-center"
			onSubmit={(event) => {
				event.preventDefault();
				if (!selection || !role || isChangePending) {
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
									? { selection: null, role: "" }
									: current,
							),
						onSettled: () => focusById(triggerId),
					},
				);
			}}
		>
			<div className="min-w-0 flex-1">
				<label htmlFor={triggerId} className="sr-only">
					User or group
				</label>
				<UserOrGroupAutocomplete
					id={triggerId}
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
			<Button type="submit" disabled={!selection || !role || isChangePending}>
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
}) => (
	<Select
		value={value}
		disabled={disabled}
		onValueChange={(role) => {
			if (role === "admin" || role === "use") {
				onValueChange(role);
			}
		}}
	>
		<SelectTrigger ref={ref} className="w-full sm:w-32" aria-label={label}>
			<SelectValue placeholder="Role">
				{value === "" ? undefined : chatProjectRoleLabel[value]}
			</SelectValue>
		</SelectTrigger>
		<SelectContent align="end">
			{(["admin", "use"] as const).map((role) => (
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

type AccessRowProps = {
	readonly entry: ProjectAccessEntry;
	readonly canManage: boolean;
	readonly isCurrentUser: boolean;
	readonly disabled: boolean;
	readonly onChangeRole: ProjectAccessDialogProps["onChangeRole"];
	readonly onRemove: ProjectAccessDialogProps["onRemove"];
	readonly onRemoved: () => void;
};

const AccessRow: React.FC<AccessRowProps> = ({
	entry,
	canManage,
	isCurrentUser,
	disabled,
	onChangeRole,
	onRemove,
	onRemoved,
}) => {
	const editableEntry =
		canManage && entry.kind !== "owner" && !isCurrentUser ? entry : undefined;
	const roleTriggerRef = useRef<HTMLButtonElement>(null);
	const menuTriggerRef = useRef<HTMLButtonElement>(null);
	// Saving disables the row; refocus the control that started the change.
	const focusAfterSave = useRef<HTMLButtonElement | null>(null);
	useEffect(() => {
		if (!disabled) {
			focusAfterSave.current?.focus();
			focusAfterSave.current = null;
		}
	}, [disabled]);

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
						ref={roleTriggerRef}
						onValueChange={(role) => {
							focusAfterSave.current = roleTriggerRef.current;
							onChangeRole(editableEntry, role, {});
						}}
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
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									size="icon-lg"
									variant="subtle"
									ref={menuTriggerRef}
									aria-label={`Open menu for ${editableEntry.name}`}
									disabled={disabled}
								>
									<EllipsisVerticalIcon aria-hidden="true" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onClick={() => {
										focusAfterSave.current = menuTriggerRef.current;
										onRemove(editableEntry, { onSuccess: onRemoved });
									}}
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
