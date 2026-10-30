import { Trash2Icon, UserPlusIcon } from "lucide-react";
import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProjectACL,
	chatProjectACLAvailable,
	updateChatProjectACL,
} from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectACL,
	ChatProjectRole,
	UpdateChatProjectACLRequest,
	UsersRequest,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
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
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { getGroupSubtitle, isGroup } from "#/modules/groups";
import {
	type ACLPrincipal,
	ACLPrincipalAutocomplete,
} from "#/pages/AISettingsPage/components/ACLPrincipalAutocomplete";

type ShareRole = Exclude<ChatProjectRole, "">;
type PrincipalKind = "user" | "group";

const roleOptions: readonly {
	readonly value: ShareRole;
	readonly label: string;
	readonly description: string;
}[] = [
	{
		value: "use",
		label: "Can use",
		description: "Can start chats in this agent.",
	},
	{
		value: "edit",
		label: "Can edit",
		description:
			"Can also change this agent's settings, instructions and connected accounts.",
	},
];

const toShareRole = (role: string): ShareRole =>
	role === "edit" ? "edit" : "use";

type RoleSelectProps = {
	readonly value: ShareRole;
	readonly label: string;
	/**
	 * Ignores changes while another change saves. The trigger stays
	 * focusable, so focus does not drop to the page.
	 */
	readonly busy?: boolean;
	readonly onValueChange: (role: ShareRole) => void;
};

const RoleSelect: React.FC<RoleSelectProps> = ({
	value,
	label,
	busy = false,
	onValueChange,
}) => {
	const [open, setOpen] = useState(false);
	return (
		<Select
			value={value}
			open={open && !busy}
			onOpenChange={(next) => setOpen(next && !busy)}
			onValueChange={(role) => {
				if (!busy) {
					onValueChange(toShareRole(role));
				}
			}}
		>
			<SelectTrigger
				aria-label={label}
				aria-disabled={busy}
				className="h-9 w-36 aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
			>
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				{roleOptions.map((option) => (
					<SelectItem
						key={option.value}
						value={option.value}
						className="w-64 flex-col items-start py-2"
					>
						<div className="text-content-primary">{option.label}</div>
						<div className="text-xs leading-[140%] text-content-secondary">
							{option.description}
						</div>
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
};

type ProjectSharingDialogViewProps = {
	readonly project: ChatProject;
	/** `undefined` until the sharing list loads. */
	readonly acl: ChatProjectACL | undefined;
	readonly loadError: unknown;
	/** Shown when the last change was rejected, for example when sharing is disabled. */
	readonly saveError: unknown;
	readonly isSaving: boolean;
	readonly availableQueryOptions: React.ComponentProps<
		typeof ACLPrincipalAutocomplete
	>["availableQueryOptions"];
	readonly onAdd: (principal: ACLPrincipal, role: ShareRole) => void;
	readonly onChangeRole: (
		kind: PrincipalKind,
		id: string,
		role: ShareRole,
	) => void;
	readonly onRemove: (kind: PrincipalKind, id: string) => void;
	readonly onClose: () => void;
};

/** Lists who an agent is shared with and edits their roles. */
export const ProjectSharingDialogView: React.FC<
	ProjectSharingDialogViewProps
> = ({
	project,
	acl,
	loadError,
	saveError,
	isSaving,
	availableQueryOptions,
	onAdd,
	onChangeRole,
	onRemove,
	onClose,
}) => {
	const [selected, setSelected] = useState<ACLPrincipal | null>(null);
	const pickerRef = useRef<HTMLDivElement>(null);
	const [newRole, setNewRole] = useState<ShareRole>("use");
	const hasEditor =
		newRole === "edit" ||
		acl?.users.some((user) => user.role === "edit") ||
		acl?.groups.some((group) => group.role === "edit");
	const excludedPrincipalIds = [
		...(acl?.users ?? []).map((user) => user.id),
		...(acl?.groups ?? []).map((group) => group.id),
	];

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open) {
					onClose();
				}
			}}
		>
			<DialogContent className="max-w-2xl">
				<DialogHeader>
					<DialogTitle>Share {project.name}</DialogTitle>
					<DialogDescription>
						People you share this agent with can start chats in it. Agents it
						invokes are shared separately.
					</DialogDescription>
				</DialogHeader>

				{Boolean(saveError) && <ErrorAlert error={saveError} />}
				{Boolean(loadError) && <ErrorAlert error={loadError} />}

				<form
					className="flex flex-col gap-2 sm:flex-row sm:items-end"
					onSubmit={(event) => {
						event.preventDefault();
						if (!selected || isSaving) {
							return;
						}
						onAdd(selected, newRole);
						setSelected(null);
						// Ready for the next person.
						pickerRef.current?.querySelector("button")?.focus();
					}}
				>
					<div ref={pickerRef} className="min-w-0 flex-1">
						<ACLPrincipalAutocomplete
							value={selected}
							onChange={setSelected}
							availableQueryOptions={availableQueryOptions}
							excludedPrincipalIds={excludedPrincipalIds}
						/>
					</div>
					<RoleSelect
						value={newRole}
						label="Role for new member"
						onValueChange={setNewRole}
					/>
					<Button
						type="submit"
						// Stays focusable while saving, so focus does not drop to the page.
						aria-disabled={!selected || isSaving}
						className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
					>
						<UserPlusIcon className="size-icon-sm" />
						Add
					</Button>
				</form>

				{hasEditor && (
					<p className="m-0 text-xs text-content-secondary">
						Editors can change this agent's configuration and connected
						accounts, which every chat in it uses, including chats started by
						triggers.
					</p>
				)}

				<Table aria-label="People and groups with access">
					<TableHeader>
						<TableRow>
							<TableHead>Member</TableHead>
							<TableHead className="w-40">Role</TableHead>
							<TableHead className="w-16">
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						<MembersRows
							acl={acl}
							hasError={Boolean(loadError)}
							isSaving={isSaving}
							onChangeRole={onChangeRole}
							onRemove={onRemove}
						/>
					</TableBody>
				</Table>

				<DialogFooter>
					<Button type="button" variant="outline" onClick={onClose}>
						Done
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

type MembersRowsProps = Pick<
	ProjectSharingDialogViewProps,
	"acl" | "isSaving" | "onChangeRole" | "onRemove"
> & { readonly hasError: boolean };

const MembersRows: React.FC<MembersRowsProps> = ({
	acl,
	hasError,
	isSaving,
	onChangeRole,
	onRemove,
}) => {
	if (!acl) {
		return hasError ? null : <TableLoader />;
	}
	if (acl.users.length === 0 && acl.groups.length === 0) {
		return (
			<TableEmpty
				message="Not shared yet"
				description="Add a person or group using the controls above."
			/>
		);
	}
	return (
		<>
			{acl.groups.map((group) => {
				const name = group.display_name || group.name;
				return (
					<TableRow key={group.id}>
						<TableCell>
							<AvatarData
								title={name}
								subtitle={getGroupSubtitle(group)}
								src={group.avatar_url}
							/>
						</TableCell>
						<TableCell>
							<RoleSelect
								value={toShareRole(group.role)}
								label={`Role for ${name}`}
								busy={isSaving}
								onValueChange={(role) => onChangeRole("group", group.id, role)}
							/>
						</TableCell>
						<TableCell>
							<Button
								variant="subtle"
								size="icon"
								type="button"
								disabled={isSaving}
								aria-label={`Remove ${name}`}
								onClick={() => onRemove("group", group.id)}
							>
								<Trash2Icon />
							</Button>
						</TableCell>
					</TableRow>
				);
			})}
			{acl.users.map((user) => {
				const shownName = user.name || user.username;
				return (
					<TableRow key={user.id}>
						<TableCell>
							<AvatarData
								title={shownName}
								subtitle={`@${user.username}`}
								src={user.avatar_url}
							/>
						</TableCell>
						<TableCell>
							<RoleSelect
								value={toShareRole(user.role)}
								label={`Role for ${shownName}`}
								busy={isSaving}
								onValueChange={(role) => onChangeRole("user", user.id, role)}
							/>
						</TableCell>
						<TableCell>
							<Button
								variant="subtle"
								size="icon"
								type="button"
								disabled={isSaving}
								aria-label={`Remove ${shownName}`}
								onClick={() => onRemove("user", user.id)}
							>
								<Trash2Icon />
							</Button>
						</TableCell>
					</TableRow>
				);
			})}
		</>
	);
};

type ProjectSharingDialogProps = {
	readonly project: ChatProject;
	readonly onClose: () => void;
};

const rolesFor = (
	kind: PrincipalKind,
	id: string,
	role: ChatProjectRole,
): UpdateChatProjectACLRequest =>
	kind === "group"
		? { group_roles: { [id]: role } }
		: { user_roles: { [id]: role } };

/** Loads an agent's sharing list and applies each change as it is made. */
export const ProjectSharingDialog: React.FC<ProjectSharingDialogProps> = ({
	project,
	onClose,
}) => {
	const queryClient = useQueryClient();
	const aclQuery = useQuery(chatProjectACL(project));
	const updateMutation = useMutation(
		updateChatProjectACL(queryClient, project),
	);

	return (
		<ProjectSharingDialogView
			project={project}
			acl={aclQuery.data}
			loadError={aclQuery.error}
			saveError={updateMutation.error}
			isSaving={updateMutation.isPending}
			availableQueryOptions={(options: UsersRequest) =>
				chatProjectACLAvailable(project, options)
			}
			onAdd={(principal, role) =>
				updateMutation.mutate(
					rolesFor(isGroup(principal) ? "group" : "user", principal.id, role),
				)
			}
			onChangeRole={(kind, id, role) =>
				updateMutation.mutate(rolesFor(kind, id, role))
			}
			onRemove={(kind, id) => updateMutation.mutate(rolesFor(kind, id, ""))}
			onClose={onClose}
		/>
	);
};
