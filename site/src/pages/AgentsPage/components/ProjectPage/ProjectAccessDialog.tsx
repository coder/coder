import { EllipsisVerticalIcon, UserPlusIcon } from "lucide-react";
import { useState } from "react";
import type { ChatProjectRole } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
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
	chatProjectRoleLabel,
	type ProjectAccessEntry,
	type ProjectAccessPrincipal,
} from "./projectAccess";

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
	/** Grants access; call `onSuccess` once the change is saved. */
	readonly onAdd: (
		principal: ProjectAccessPrincipal,
		role: AssignableRole,
		onSuccess: () => void,
	) => void;
	readonly onChangeRole: (
		principal: ProjectAccessPrincipal,
		role: AssignableRole,
	) => void;
	readonly onRemove: (principal: ProjectAccessPrincipal) => void;
	/** The user or group whose change is saving, including one being added. */
	readonly pendingPrincipalId: string | undefined;
	readonly mutationError: unknown;
};

/** Lists who can use a project, and lets owners and admins change it; onCloseAutoFocus restores focus. */
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
						organizationId={organizationId}
						// Owners and existing entries cannot be added, and the
						// viewer cannot change their own access.
						exclude={[...entries, { id: currentUserId }]}
						pendingPrincipalId={pendingPrincipalId}
						onAdd={onAdd}
					/>
				)}
				{Boolean(mutationError) && <ErrorAlert error={mutationError} />}
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
					<Table aria-label="Project access">
						<TableHeader>
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
						<TableBody>
							{entries ? (
								entries.map((entry) => (
									<AccessRow
										key={`${entry.kind}:${entry.id}`}
										entry={entry}
										canManage={canManage}
										isCurrentUser={
											entry.kind === "user" && entry.id === currentUserId
										}
										isPending={entry.id === pendingPrincipalId}
										onChangeRole={onChangeRole}
										onRemove={onRemove}
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

type AddAccessFormProps = {
	readonly organizationId: string;
	readonly exclude: { readonly id: string }[];
	readonly pendingPrincipalId: string | undefined;
	readonly onAdd: ProjectAccessDialogProps["onAdd"];
};

const AddAccessForm: React.FC<AddAccessFormProps> = ({
	organizationId,
	exclude,
	pendingPrincipalId,
	onAdd,
}) => {
	const [selection, setSelection] =
		useState<UserOrGroupAutocompleteValue>(null);
	const [role, setRole] = useState<AssignableRole | "">("");
	// The selection is never listed yet, so a pending change for it is this add.
	const isAdding = selection !== null && selection.id === pendingPrincipalId;

	return (
		<form
			className="flex flex-col gap-2 sm:flex-row sm:items-center"
			onSubmit={(event) => {
				event.preventDefault();
				if (!selection || !role || isAdding) {
					return;
				}
				onAdd(
					{ kind: isGroup(selection) ? "group" : "user", id: selection.id },
					role,
					() => {
						setSelection(null);
						setRole("");
					},
				);
			}}
		>
			<div className="min-w-0 flex-1">
				<UserOrGroupAutocomplete
					value={selection}
					onChange={setSelection}
					organizationId={organizationId}
					exclude={exclude}
					className="w-full"
				/>
			</div>
			<ProjectRoleSelect
				value={role}
				label="Role"
				disabled={isAdding}
				onValueChange={setRole}
			/>
			<Button type="submit" disabled={!selection || !role || isAdding}>
				<Spinner loading={isAdding}>
					<UserPlusIcon className="size-icon-sm" />
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
};

const ProjectRoleSelect: React.FC<ProjectRoleSelectProps> = ({
	value,
	label,
	disabled,
	onValueChange,
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
		<SelectTrigger className="w-full sm:w-32" aria-label={label}>
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
	readonly isPending: boolean;
	readonly onChangeRole: ProjectAccessDialogProps["onChangeRole"];
	readonly onRemove: ProjectAccessDialogProps["onRemove"];
};

const AccessRow: React.FC<AccessRowProps> = ({
	entry,
	canManage,
	isCurrentUser,
	isPending,
	onChangeRole,
	onRemove,
}) => {
	const editableEntry =
		canManage && entry.kind !== "owner" && !isCurrentUser ? entry : undefined;

	return (
		<TableRow>
			<TableCell>
				{entry.name === undefined ? (
					<>
						<AvatarDataSkeleton />
						<span className="sr-only">Loading</span>
					</>
				) : (
					<AvatarData
						title={entry.name}
						subtitle={entry.subtitle}
						src={entry.avatarUrl}
					/>
				)}
			</TableCell>
			<TableCell>
				{editableEntry ? (
					<ProjectRoleSelect
						value={editableEntry.role}
						label={`Role for ${editableEntry.name}`}
						disabled={isPending}
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
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									size="icon-lg"
									variant="subtle"
									aria-label={`Open menu for ${editableEntry.name}`}
									disabled={isPending}
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
