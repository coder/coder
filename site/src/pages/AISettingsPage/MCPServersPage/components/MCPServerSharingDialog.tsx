import isEqual from "lodash/isEqual";
import { Trash2Icon, UserPlusIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	mcpServerConfigACL,
	updateMCPServerConfigACL,
} from "#/api/queries/chats";
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
	MCPServerPrincipalAutocomplete,
	type MCPServerPrincipalAutocompleteValue,
} from "./MCPServerPrincipalAutocomplete";

type MCPServerSharingDialogProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	organizationId: string;
	serverId: string;
	serverName: string;
};

type DraftACL = {
	user_roles: Record<string, TypesGen.MCPServerConfigRole>;
	group_roles: Record<string, TypesGen.MCPServerConfigRole>;
};

const emptyDraft = (): DraftACL => ({ user_roles: {}, group_roles: {} });

const buildRoleDelta = (
	initial: Record<string, TypesGen.MCPServerConfigRole>,
	current: Record<string, TypesGen.MCPServerConfigRole>,
): Record<string, TypesGen.MCPServerConfigRole> => {
	const delta: Record<string, TypesGen.MCPServerConfigRole> = {};
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
): TypesGen.UpdateMCPServerConfigACLRequest => {
	const userRoles = buildRoleDelta(initial.user_roles, current.user_roles);
	const groupRoles = buildRoleDelta(initial.group_roles, current.group_roles);
	return {
		...(Object.keys(userRoles).length > 0 ? { user_roles: userRoles } : {}),
		...(Object.keys(groupRoles).length > 0 ? { group_roles: groupRoles } : {}),
	};
};

export const MCPServerSharingDialog: FC<MCPServerSharingDialogProps> = ({
	open,
	onOpenChange,
	organizationId,
	serverId,
	serverName,
}) => {
	const queryClient = useQueryClient();
	const [draft, setDraft] = useState<DraftACL>(emptyDraft);
	const [initialACL, setInitialACL] = useState<DraftACL | null>(null);
	const [userIdentities, setUserIdentities] = useState<
		Record<string, TypesGen.MCPServerConfigUser | TypesGen.MinimalUser>
	>({});
	const [groupIdentities, setGroupIdentities] = useState<
		Record<string, TypesGen.Group>
	>({});
	const [initialized, setInitialized] = useState(false);
	const [selectedOption, setSelectedOption] =
		useState<MCPServerPrincipalAutocompleteValue>(null);

	const aclQuery = useQuery({
		...mcpServerConfigACL(organizationId, serverId),
		enabled: open && initialized,
	});
	const updateMutation = useMutation(updateMCPServerConfigACL(queryClient));

	useEffect(() => {
		if (!open || initialized) {
			return;
		}
		let cancelled = false;
		void aclQuery.refetch().then(({ data, isError }) => {
			if (cancelled || isError || !data) {
				return;
			}
			const snapshot = {
				user_roles: Object.fromEntries(
					data.users.map((user) => [user.id, user.role]),
				),
				group_roles: Object.fromEntries(
					data.groups.map((group) => [group.id, group.role]),
				),
			};
			setInitialACL(snapshot);
			setDraft(snapshot);
			setUserIdentities(
				Object.fromEntries(data.users.map((user) => [user.id, user])),
			);
			setGroupIdentities(
				Object.fromEntries(data.groups.map((group) => [group.id, group])),
			);
			setInitialized(true);
		});
		return () => {
			cancelled = true;
		};
	}, [aclQuery.refetch, initialized, open]);

	const reset = () => {
		setDraft(emptyDraft());
		setInitialACL(null);
		setUserIdentities({});
		setGroupIdentities({});
		setInitialized(false);
		setSelectedOption(null);
		updateMutation.reset();
	};
	const close = () => {
		reset();
		onOpenChange(false);
	};

	const userIds = Object.keys(draft.user_roles);
	const groupIds = Object.keys(draft.group_roles);
	const excludedPrincipalIds = [...userIds, ...groupIds];
	const loadError = !initialized ? aclQuery.error : null;
	const refetchError = initialized ? aclQuery.error : null;
	const isLoading = !loadError && open && !initialized;
	const isEmpty = userIds.length === 0 && groupIds.length === 0;
	const isDirty =
		initialized && initialACL !== null && !isEqual(draft, initialACL);

	const addSelectedPrincipal = () => {
		if (!selectedOption) {
			return;
		}
		if (isGroup(selectedOption)) {
			setGroupIdentities((current) => ({
				...current,
				[selectedOption.id]: selectedOption,
			}));
			setDraft((current) => ({
				...current,
				group_roles: { ...current.group_roles, [selectedOption.id]: "read" },
			}));
		} else {
			setUserIdentities((current) => ({
				...current,
				[selectedOption.id]: selectedOption,
			}));
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
				organization: organizationId,
				id: serverId,
				req: buildACLDelta(initialACL, draft),
			},
			{
				onSuccess: () => {
					toast.success(`Sharing for "${serverName}" updated.`);
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
					<DialogTitle>Share server</DialogTitle>
					<DialogDescription>
						Choose which organization members and groups can use {serverName}.
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
						<span>Loading server sharing</span>
					</div>
				) : !initialized ? null : (
					<div className="flex flex-col gap-4">
						<form
							action={addSelectedPrincipal}
							className="flex flex-col gap-2 sm:flex-row sm:items-center"
						>
							<div className="min-w-0 flex-1">
								<MCPServerPrincipalAutocomplete
									organizationId={organizationId}
									value={selectedOption}
									onChange={setSelectedOption}
									serverId={serverId}
									excludedPrincipalIds={excludedPrincipalIds}
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
									No shared members or groups yet
								</p>
								<p className="m-0 mt-2 text-sm text-content-secondary">
									Add a member or group using the controls above.
								</p>
							</div>
						) : (
							<Table aria-label="Shared server members and groups">
								<TableHeader>
									<TableRow>
										<TableHead>Member</TableHead>
										<TableHead className="w-24">Role</TableHead>
										<TableHead className="w-16" />
									</TableRow>
								</TableHeader>
								<TableBody>
									{groupIds.map((groupId) => {
										const group = groupIdentities[groupId];
										const name =
											group?.display_name || group?.name || "Unknown group";
										return (
											<TableRow key={groupId}>
												<TableCell>
													<AvatarData
														title={name}
														subtitle={group ? getGroupSubtitle(group) : "Group"}
														src={group?.avatar_url}
													/>
												</TableCell>
												<TableCell>Read</TableCell>
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
										const user = userIdentities[userId];
										const name = user?.username || "Unknown user";
										return (
											<TableRow key={userId}>
												<TableCell>
													<AvatarData
														title={name}
														subtitle={user?.name || "User"}
														src={user?.avatar_url}
													/>
												</TableCell>
												<TableCell>Read</TableCell>
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
						confirmText="Save sharing"
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
