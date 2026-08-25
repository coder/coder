import isEqual from "lodash/isEqual";
import { Trash2Icon, UserPlusIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { chatModelACL, updateChatModelACL } from "#/api/queries/chats";
import { groupsByOrganization } from "#/api/queries/groups";
import { organizationMembers } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
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
import { getGroupSubtitle, isGroup } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";

type ChatModelSharingDialogProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	organizationId: string;
	modelId: string;
	modelName: string;
};

type DraftACL = {
	user_roles: Record<string, TypesGen.ChatRole>;
	group_roles: Record<string, TypesGen.ChatRole>;
};

const emptyDraft = (): DraftACL => ({ user_roles: {}, group_roles: {} });

const buildRoleDelta = (
	initial: Record<string, TypesGen.ChatRole>,
	current: Record<string, TypesGen.ChatRole>,
): Record<string, TypesGen.ChatRole> => {
	const delta: Record<string, TypesGen.ChatRole> = {};
	for (const [principalId, role] of Object.entries(current)) {
		if (initial[principalId] !== role) {
			delta[principalId] = role;
		}
	}
	for (const principalId of Object.keys(initial)) {
		if (current[principalId] === undefined) {
			delta[principalId] = "";
		}
	}
	return delta;
};

const buildACLDelta = (
	initial: DraftACL,
	current: DraftACL,
): TypesGen.UpdateChatModelACLRequest => {
	const userRoles = buildRoleDelta(initial.user_roles, current.user_roles);
	const groupRoles = buildRoleDelta(initial.group_roles, current.group_roles);
	return {
		...(Object.keys(userRoles).length > 0 ? { user_roles: userRoles } : {}),
		...(Object.keys(groupRoles).length > 0 ? { group_roles: groupRoles } : {}),
	};
};

export const ChatModelSharingDialog: FC<ChatModelSharingDialogProps> = ({
	open,
	onOpenChange,
	organizationId,
	modelId,
	modelName,
}) => {
	const queryClient = useQueryClient();
	const [draft, setDraft] = useState<DraftACL>(emptyDraft);
	const [initialACL, setInitialACL] = useState<DraftACL | null>(null);
	const [initialized, setInitialized] = useState(false);
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);

	const aclQuery = useQuery({
		...chatModelACL(organizationId, modelId),
		enabled: open && initialized,
	});
	const membersQuery = useQuery({
		...organizationMembers(organizationId, { limit: 0 }),
		enabled: open,
	});
	const groupsQuery = useQuery({
		...groupsByOrganization(organizationId),
		enabled: open,
	});
	const updateMutation = useMutation(updateChatModelACL(queryClient));

	useEffect(() => {
		if (!open || initialized) {
			return;
		}
		let cancelled = false;
		void aclQuery.refetch().then(({ data }) => {
			if (cancelled || !data) {
				return;
			}
			const snapshot = {
				user_roles: { ...data.user_roles },
				group_roles: { ...data.group_roles },
			};
			setInitialACL(snapshot);
			setDraft(snapshot);
			setInitialized(true);
		});
		return () => {
			cancelled = true;
		};
	}, [aclQuery.refetch, initialized, open]);

	const reset = () => {
		setDraft(emptyDraft());
		setInitialACL(null);
		setInitialized(false);
		setSelectedOption(null);
		updateMutation.reset();
	};
	const close = () => {
		reset();
		onOpenChange(false);
	};

	const members = membersQuery.data?.members ?? [];
	const groups = groupsQuery.data ?? [];
	const userIds = Object.keys(draft.user_roles);
	const groupIds = Object.keys(draft.group_roles);
	const excludedPrincipals = [
		...members
			.filter((member) => userIds.includes(member.user_id))
			.map((member) => ({ id: member.user_id })),
		...groups.filter((group) => groupIds.includes(group.id)),
	];
	const loadError =
		(!initialized ? aclQuery.error : null) ??
		(membersQuery.data === undefined ? membersQuery.error : null) ??
		(groupsQuery.data === undefined ? groupsQuery.error : null);
	const refetchError = loadError
		? null
		: (aclQuery.error ?? membersQuery.error ?? groupsQuery.error);
	const isLoading =
		!loadError &&
		((open && !initialized) || membersQuery.isLoading || groupsQuery.isLoading);
	const isEmpty = userIds.length === 0 && groupIds.length === 0;
	const isDirty =
		initialized && initialACL !== null && !isEqual(draft, initialACL);

	const addSelectedPrincipal = () => {
		if (!selectedOption) {
			return;
		}
		if (isGroup(selectedOption)) {
			setDraft((current) => ({
				...current,
				group_roles: { ...current.group_roles, [selectedOption.id]: "read" },
			}));
		} else {
			setDraft((current) => ({
				...current,
				user_roles: { ...current.user_roles, [selectedOption.id]: "read" },
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

	const save = () => {
		if (!initialACL) {
			return;
		}
		updateMutation.mutate(
			{
				organizationId,
				modelId,
				req: buildACLDelta(initialACL, draft),
			},
			{
				onSuccess: () => {
					toast.success(`Permissions for "${modelName}" updated.`);
					close();
				},
			},
		);
	};

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen && !updateMutation.isPending) {
					close();
				}
			}}
		>
			<DialogContent className="max-w-2xl">
				<DialogHeader>
					<DialogTitle>Model permissions</DialogTitle>
					<DialogDescription>
						Manage which organization members and groups can use {modelName}.
					</DialogDescription>
				</DialogHeader>

				{updateMutation.error && <ErrorAlert error={updateMutation.error} />}
				{refetchError && <ErrorAlert error={refetchError} />}
				{loadError && <ErrorAlert error={loadError} />}

				{loadError ? null : isLoading ? (
					<div
						role="status"
						className="flex items-center justify-center gap-3 py-10"
					>
						<Spinner loading />
						<span>Loading model permissions</span>
					</div>
				) : !initialized ? null : (
					<div className="flex flex-col gap-4">
						<form
							action={addSelectedPrincipal}
							className="flex flex-col gap-2 sm:flex-row sm:items-center"
						>
							<div className="min-w-0 flex-1">
								<UserOrGroupAutocomplete
									organizationId={organizationId}
									value={selectedOption}
									onChange={setSelectedOption}
									exclude={excludedPrincipals}
									className="w-full"
								/>
							</div>
							<Button
								type="submit"
								disabled={!selectedOption || updateMutation.isPending}
							>
								<UserPlusIcon className="size-icon-sm" />
								Add member
							</Button>
						</form>

						{isEmpty ? (
							<div className="rounded-md border border-solid border-border px-6 py-10 text-center">
								<p className="m-0 text-sm font-medium">
									No members or groups have permission yet
								</p>
								<p className="m-0 mt-2 text-sm text-content-secondary">
									Add a member or group using the controls above.
								</p>
							</div>
						) : (
							<Table aria-label="Model permissions for members and groups">
								<TableHeader>
									<TableRow>
										<TableHead>Member</TableHead>
										<TableHead className="w-24">Role</TableHead>
										<TableHead className="w-16" />
									</TableRow>
								</TableHeader>
								<TableBody>
									{groupIds.map((groupId) => {
										const group = groups.find((item) => item.id === groupId);
										const name = group?.display_name || group?.name || groupId;
										return (
											<TableRow key={groupId}>
												<TableCell>
													<AvatarData
														title={name}
														subtitle={group ? getGroupSubtitle(group) : "Group"}
														src={group?.avatar_url}
													/>
												</TableCell>
												<TableCell>Use</TableCell>
												<TableCell>
													<Button
														variant="subtle"
														size="icon"
														type="button"
														disabled={updateMutation.isPending}
														aria-label={`Remove ${name}`}
														onClick={() => removeGroup(groupId)}
													>
														<Trash2Icon />
													</Button>
												</TableCell>
											</TableRow>
										);
									})}
									{userIds.map((userId) => {
										const member = members.find(
											(item) => item.user_id === userId,
										);
										const name = member?.username || userId;
										return (
											<TableRow key={userId}>
												<TableCell>
													<AvatarData
														title={name}
														subtitle={member?.name || member?.email || "User"}
														src={member?.avatar_url}
													/>
												</TableCell>
												<TableCell>Use</TableCell>
												<TableCell>
													<Button
														variant="subtle"
														size="icon"
														type="button"
														disabled={updateMutation.isPending}
														aria-label={`Remove ${name}`}
														onClick={() => removeUser(userId)}
													>
														<Trash2Icon />
													</Button>
												</TableCell>
											</TableRow>
										);
									})}
								</TableBody>
							</Table>
						)}
					</div>
				)}

				<DialogFooter>
					<DialogActions
						confirmText="Save permissions"
						confirmLoading={updateMutation.isPending}
						confirmDisabled={isLoading || Boolean(loadError) || !isDirty}
						onConfirm={save}
						onCancel={close}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
