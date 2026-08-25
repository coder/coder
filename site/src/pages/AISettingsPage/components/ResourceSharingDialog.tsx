import isEqual from "lodash/isEqual";
import { Trash2Icon, UserPlusIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";

type SharingACL<Role extends string> = {
	user_roles: Record<string, Role>;
	group_roles: Record<string, Role>;
};

type SharingACLUpdate<Role extends string> = {
	user_roles?: Record<string, Role>;
	group_roles?: Record<string, Role>;
};

export type SharingPrincipal = {
	id: string;
	name: string;
	subtitle: string;
	avatarUrl?: string;
};

type SharingPrincipals = {
	users: Record<string, SharingPrincipal>;
	groups: Record<string, SharingPrincipal>;
};

export type SharingDialogData<Role extends string> = {
	acl: SharingACL<Role>;
	principals: SharingPrincipals;
};

export type SharingPrincipalSelection = {
	kind: "user" | "group";
	principal: SharingPrincipal;
};

type SharingAutocompleteProps<Option> = {
	value: Option | null;
	onChange: (value: Option | null) => void;
	excludedPrincipalIds: readonly string[];
};

type ResourceSharingDialogProps<Role extends string, Option> = {
	title: string;
	description: ReactNode;
	loadingLabel: string;
	tableLabel: string;
	data?: SharingDialogData<Role>;
	loadError: unknown;
	refetchError: unknown;
	saveError: unknown;
	isSaving: boolean;
	readRole: Role;
	deletedRole: Role;
	renderAutocomplete: (props: SharingAutocompleteProps<Option>) => ReactNode;
	getPrincipal: (option: Option) => SharingPrincipalSelection;
	onClose: () => void;
	onSave: (update: SharingACLUpdate<Role>) => void;
};

const buildRoleDelta = <Role extends string>(
	initial: Record<string, Role>,
	current: Record<string, Role>,
	deletedRole: Role,
): Record<string, Role> => {
	const delta: Record<string, Role> = {};
	for (const [principalId, role] of Object.entries(current)) {
		if (initial[principalId] !== role) {
			delta[principalId] = role;
		}
	}
	for (const principalId of Object.keys(initial)) {
		if (current[principalId] === undefined) {
			delta[principalId] = deletedRole;
		}
	}
	return delta;
};

const buildACLDelta = <Role extends string>(
	initial: SharingACL<Role>,
	current: SharingACL<Role>,
	deletedRole: Role,
): SharingACLUpdate<Role> => {
	const userRoles = buildRoleDelta(
		initial.user_roles,
		current.user_roles,
		deletedRole,
	);
	const groupRoles = buildRoleDelta(
		initial.group_roles,
		current.group_roles,
		deletedRole,
	);
	return {
		...(Object.keys(userRoles).length > 0 ? { user_roles: userRoles } : {}),
		...(Object.keys(groupRoles).length > 0 ? { group_roles: groupRoles } : {}),
	};
};

type PrincipalRowProps = {
	principal: SharingPrincipal;
	isSaving: boolean;
	onRemove: () => void;
};

const PrincipalRow: FC<PrincipalRowProps> = ({
	principal,
	isSaving,
	onRemove,
}) => (
	<TableRow>
		<TableCell>
			<AvatarData
				title={principal.name}
				subtitle={principal.subtitle}
				src={principal.avatarUrl}
			/>
		</TableCell>
		<TableCell>Read</TableCell>
		<TableCell>
			<Button
				variant="subtle"
				size="icon"
				type="button"
				disabled={isSaving}
				aria-label={`Remove ${principal.name}`}
				onClick={onRemove}
			>
				<Trash2Icon />
			</Button>
		</TableCell>
	</TableRow>
);

type SharingDialogEditorProps<Role extends string, Option> = Pick<
	ResourceSharingDialogProps<Role, Option>,
	| "data"
	| "deletedRole"
	| "getPrincipal"
	| "isSaving"
	| "onClose"
	| "onSave"
	| "readRole"
	| "renderAutocomplete"
	| "tableLabel"
>;

const SharingDialogEditor = <Role extends string, Option>({
	data,
	deletedRole,
	getPrincipal,
	isSaving,
	onClose,
	onSave,
	readRole,
	renderAutocomplete,
	tableLabel,
}: SharingDialogEditorProps<Role, Option>) => {
	if (!data) {
		return null;
	}
	return (
		<LoadedSharingDialogEditor
			data={data}
			deletedRole={deletedRole}
			getPrincipal={getPrincipal}
			isSaving={isSaving}
			onClose={onClose}
			onSave={onSave}
			readRole={readRole}
			renderAutocomplete={renderAutocomplete}
			tableLabel={tableLabel}
		/>
	);
};

const LoadedSharingDialogEditor = <Role extends string, Option>({
	data,
	deletedRole,
	getPrincipal,
	isSaving,
	onClose,
	onSave,
	readRole,
	renderAutocomplete,
	tableLabel,
}: SharingDialogEditorProps<Role, Option> & {
	data: SharingDialogData<Role>;
}) => {
	const [initialACL] = useState(data.acl);
	const [draft, setDraft] = useState(data.acl);
	const [principals, setPrincipals] = useState(data.principals);
	const [selectedOption, setSelectedOption] = useState<Option | null>(null);

	const userIds = Object.keys(draft.user_roles);
	const groupIds = Object.keys(draft.group_roles);
	const excludedPrincipalIds = [...userIds, ...groupIds];
	const isEmpty = userIds.length === 0 && groupIds.length === 0;
	const isDirty = !isEqual(draft, initialACL);

	const addSelectedPrincipal = () => {
		if (!selectedOption) {
			return;
		}
		const selection = getPrincipal(selectedOption);
		if (selection.kind === "group") {
			setPrincipals((current) => ({
				...current,
				groups: {
					...current.groups,
					[selection.principal.id]: selection.principal,
				},
			}));
			setDraft((current) => ({
				...current,
				group_roles: {
					...current.group_roles,
					[selection.principal.id]: readRole,
				},
			}));
		} else {
			setPrincipals((current) => ({
				...current,
				users: {
					...current.users,
					[selection.principal.id]: selection.principal,
				},
			}));
			setDraft((current) => ({
				...current,
				user_roles: {
					...current.user_roles,
					[selection.principal.id]: readRole,
				},
			}));
		}
		setSelectedOption(null);
	};

	const removeUser = (userId: string) => {
		setDraft((current) => {
			const userRoles = { ...current.user_roles };
			delete userRoles[userId];
			return { ...current, user_roles: userRoles };
		});
	};
	const removeGroup = (groupId: string) => {
		setDraft((current) => {
			const groupRoles = { ...current.group_roles };
			delete groupRoles[groupId];
			return { ...current, group_roles: groupRoles };
		});
	};

	return (
		<>
			<div className="flex flex-col gap-4">
				<form
					action={addSelectedPrincipal}
					className="flex flex-col gap-2 sm:flex-row sm:items-center"
				>
					<div className="min-w-0 flex-1">
						{renderAutocomplete({
							value: selectedOption,
							onChange: setSelectedOption,
							excludedPrincipalIds,
						})}
					</div>
					<Button type="submit" disabled={!selectedOption || isSaving}>
						<UserPlusIcon className="size-icon-sm" />
						Add member
					</Button>
				</form>

				{isEmpty ? (
					<div className="rounded-md border border-solid border-border px-6 py-10 text-center">
						<p className="m-0 text-sm font-medium">
							No shared members or groups yet
						</p>
						<p className="m-0 mt-2 text-sm text-content-secondary">
							Add a member or group using the controls above.
						</p>
					</div>
				) : (
					<Table aria-label={tableLabel}>
						<TableHeader>
							<TableRow>
								<TableHead>Member</TableHead>
								<TableHead className="w-24">Role</TableHead>
								<TableHead className="w-16" />
							</TableRow>
						</TableHeader>
						<TableBody>
							{groupIds.map((groupId) => (
								<PrincipalRow
									key={groupId}
									principal={
										principals.groups[groupId] ?? {
											id: groupId,
											name: groupId,
											subtitle: "Group",
										}
									}
									isSaving={isSaving}
									onRemove={() => removeGroup(groupId)}
								/>
							))}
							{userIds.map((userId) => (
								<PrincipalRow
									key={userId}
									principal={
										principals.users[userId] ?? {
											id: userId,
											name: userId,
											subtitle: "User",
										}
									}
									isSaving={isSaving}
									onRemove={() => removeUser(userId)}
								/>
							))}
						</TableBody>
					</Table>
				)}
			</div>

			<DialogFooter>
				<DialogActions
					confirmText="Save sharing"
					confirmLoading={isSaving}
					confirmDisabled={!isDirty}
					onConfirm={() =>
						onSave(buildACLDelta(initialACL, draft, deletedRole))
					}
					onCancel={onClose}
				/>
			</DialogFooter>
		</>
	);
};

export const ResourceSharingDialog = <Role extends string, Option>({
	title,
	description,
	loadingLabel,
	tableLabel,
	data,
	loadError,
	refetchError,
	saveError,
	isSaving,
	readRole,
	deletedRole,
	renderAutocomplete,
	getPrincipal,
	onClose,
	onSave,
}: ResourceSharingDialogProps<Role, Option>) => (
	<Dialog
		open
		onOpenChange={(nextOpen) => {
			if (!nextOpen && !isSaving) {
				onClose();
			}
		}}
	>
		<DialogContent className="max-w-2xl">
			<DialogHeader>
				<DialogTitle>{title}</DialogTitle>
				<DialogDescription>{description}</DialogDescription>
			</DialogHeader>

			{saveError ? <ErrorAlert error={saveError} /> : null}
			{refetchError ? <ErrorAlert error={refetchError} /> : null}
			{loadError ? <ErrorAlert error={loadError} /> : null}

			{loadError ? null : data ? (
				<SharingDialogEditor
					data={data}
					deletedRole={deletedRole}
					getPrincipal={getPrincipal}
					isSaving={isSaving}
					onClose={onClose}
					onSave={onSave}
					readRole={readRole}
					renderAutocomplete={renderAutocomplete}
					tableLabel={tableLabel}
				/>
			) : (
				<div
					role="status"
					className="flex items-center justify-center gap-3 py-10"
				>
					<Spinner loading />
					<span>{loadingLabel}</span>
				</div>
			)}

			{!data && (
				<DialogFooter>
					<DialogActions
						confirmText="Save sharing"
						confirmLoading={isSaving}
						confirmDisabled
						onConfirm={onClose}
						onCancel={onClose}
					/>
				</DialogFooter>
			)}
		</DialogContent>
	</Dialog>
);
