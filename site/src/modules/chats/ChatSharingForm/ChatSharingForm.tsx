import { UserPlusIcon, XIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { ChatACL, ChatGroup, ChatUser, Group } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
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
import { getGroupSubtitle, isGroup } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";

type ChatSharingFormProps = {
	organizationId: string;
	chatACL: ChatACL | undefined;
	canUpdatePermissions: boolean;
	error: unknown;
	isMutating: boolean;
	onAddUser: (user: ChatUser) => unknown;
	onAddGroup: (group: ChatGroup | Group) => unknown;
	onRemoveUser: (user: ChatUser) => unknown;
	onRemoveGroup: (group: ChatGroup) => unknown;
	updatingUserId?: string;
	updatingGroupId?: string;
};

export const ChatSharingForm: FC<ChatSharingFormProps> = ({
	organizationId,
	chatACL,
	canUpdatePermissions,
	error,
	isMutating,
	onAddUser,
	onAddGroup,
	onRemoveUser,
	onRemoveGroup,
	updatingUserId,
	updatingGroupId,
}) => {
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);

	const excludeFromAutocomplete = chatACL
		? [...chatACL.groups, ...chatACL.users]
		: [];

	const handleAdd = async () => {
		if (!selectedOption) return;
		if (isGroup(selectedOption)) {
			await onAddGroup(selectedOption);
		} else {
			// OrganizationMember has .id assigned from user_id in the
			// autocomplete. Treat it as a ChatUser by projecting the
			// MinimalUser fields we need.
			const user: ChatUser = {
				id: selectedOption.id,
				username: selectedOption.username,
				name: selectedOption.name ?? "",
				avatar_url: selectedOption.avatar_url,
				role: "read",
			};
			await onAddUser(user);
		}
		setSelectedOption(null);
	};

	const isEmpty = Boolean(
		chatACL && chatACL.users.length === 0 && chatACL.groups.length === 0,
	);

	return (
		<div className="flex flex-col gap-4">
			{Boolean(error) && <ErrorAlert error={error} />}
			{canUpdatePermissions && (
				<div className="flex flex-row items-center gap-2">
					<UserOrGroupAutocomplete
						organizationId={organizationId}
						value={selectedOption}
						exclude={excludeFromAutocomplete}
						onChange={setSelectedOption}
					/>
					<Button
						disabled={!selectedOption || isMutating}
						onClick={handleAdd}
						data-testid="chat-share-add-button"
					>
						<Spinner loading={isMutating}>
							<UserPlusIcon className="size-icon-sm" />
						</Spinner>
						Add
					</Button>
				</div>
			)}

			<div>
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead className="w-[60%] py-2">Member</TableHead>
							<TableHead className="w-[30%] py-2">Role</TableHead>
							<TableHead className="w-[10%] py-2" />
						</TableRow>
					</TableHeader>
					<TableBody>
						{!chatACL ? (
							<TableLoader />
						) : isEmpty ? (
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState
										message="Not shared yet"
										description="Add a user or group above to share this chat."
										isCompact
									/>
								</TableCell>
							</TableRow>
						) : (
							<>
								{chatACL.groups.map((group) => (
									<TableRow key={group.id}>
										<TableCell className="py-2 w-[60%]">
											<AvatarData
												avatar={
													<Avatar
														size="lg"
														fallback={group.display_name || group.name}
														src={group.avatar_url}
													/>
												}
												title={group.display_name || group.name}
												subtitle={getGroupSubtitle(group)}
											/>
										</TableCell>
										<TableCell className="py-2 w-[30%]">
											<Badge size="sm" variant="default">
												Read
											</Badge>
										</TableCell>
										<TableCell className="py-2 w-[10%]">
											{canUpdatePermissions && (
												<Button
													size="icon-lg"
													variant="subtle"
													aria-label={`Remove ${group.display_name || group.name}`}
													disabled={updatingGroupId === group.id || isMutating}
													onClick={() => onRemoveGroup(group)}
												>
													<XIcon aria-hidden="true" />
												</Button>
											)}
										</TableCell>
									</TableRow>
								))}

								{chatACL.users.map((user) => (
									<TableRow key={user.id}>
										<TableCell className="py-2 w-[60%]">
											<AvatarData
												title={user.username}
												subtitle={user.name}
												src={user.avatar_url}
											/>
										</TableCell>
										<TableCell className="py-2 w-[30%]">
											<Badge size="sm" variant="default">
												Read
											</Badge>
										</TableCell>
										<TableCell className="py-2 w-[10%]">
											{canUpdatePermissions && (
												<Button
													size="icon-lg"
													variant="subtle"
													aria-label={`Remove ${user.username}`}
													disabled={updatingUserId === user.id || isMutating}
													onClick={() => onRemoveUser(user)}
												>
													<XIcon aria-hidden="true" />
												</Button>
											)}
										</TableCell>
									</TableRow>
								))}
							</>
						)}
					</TableBody>
				</Table>
			</div>
		</div>
	);
};
