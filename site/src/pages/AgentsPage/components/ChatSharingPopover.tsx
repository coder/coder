import { EllipsisVerticalIcon, Share2Icon, UserPlusIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	chatACL,
	setChatGroupRole,
	setChatUserRole,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { TopbarButton } from "#/components/FullPageLayout/Topbar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { getGroupSubtitle, isGroup } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";

type ChatShareButtonProps = {
	chatId: string;
	organizationId: string;
};

type ChatSharingPopoverContentProps = ChatShareButtonProps & {
	open: boolean;
};

type MemberRowMenuProps = {
	disabled: boolean;
	onRemove: () => void;
};

const ReadRoleBadge: FC = () => (
	<span className="inline-block shrink-0 rounded-md bg-surface-secondary px-2 py-0.5 text-xs leading-5">
		Read
	</span>
);

const MemberRowMenu: FC<MemberRowMenuProps> = ({ disabled, onRemove }) => (
	<DropdownMenu>
		<DropdownMenuTrigger asChild>
			<Button
				size="icon-lg"
				variant="subtle"
				aria-label="Open menu"
				disabled={disabled}
			>
				<EllipsisVerticalIcon aria-hidden="true" />
			</Button>
		</DropdownMenuTrigger>
		<DropdownMenuContent align="end">
			<DropdownMenuItem
				className="text-content-destructive focus:text-content-destructive"
				onClick={onRemove}
			>
				Remove
			</DropdownMenuItem>
		</DropdownMenuContent>
	</DropdownMenu>
);

type AddChatMemberFormProps = {
	isLoading: boolean;
	onSubmit: () => void;
	disabled: boolean;
	children: ReactNode;
};

const AddChatMemberForm: FC<AddChatMemberFormProps> = ({
	isLoading,
	onSubmit,
	disabled,
	children,
}) => (
	<form action={onSubmit}>
		<div className="flex flex-col gap-2 sm:flex-row sm:items-center">
			<div className="min-w-0 flex-1">{children}</div>
			<Button
				disabled={disabled || isLoading}
				type="submit"
				className="w-full sm:w-auto"
			>
				<Spinner loading={isLoading}>
					<UserPlusIcon className="size-icon-sm" />
				</Spinner>
				Add member
			</Button>
		</div>
	</form>
);

type MemberIdentityProps =
	| { kind: "group"; group: TypesGen.ChatGroup }
	| { kind: "user"; user: TypesGen.ChatUser };

const MemberIdentity: FC<MemberIdentityProps> = (props) => {
	if (props.kind === "group") {
		const { group } = props;
		return (
			<AvatarData
				title={group.display_name || group.name}
				subtitle={getGroupSubtitle(group)}
				src={group.avatar_url}
				avatar={
					<Avatar
						src={group.avatar_url}
						fallback={group.display_name || group.name}
						variant="icon"
					/>
				}
			/>
		);
	}

	const { user } = props;
	return (
		<AvatarData
			title={user.username}
			subtitle={user.name}
			src={user.avatar_url}
		/>
	);
};

type MobileMemberRowProps = {
	children: ReactNode;
	disabled: boolean;
	onRemove: () => void;
};

const MobileMemberRow: FC<MobileMemberRowProps> = ({
	children,
	disabled,
	onRemove,
}) => (
	<div className="flex items-center justify-between gap-3 border-0 border-b border-solid border-border last:border-b-0 px-1 py-3">
		<div className="min-w-0 flex-1">{children}</div>
		<div className="flex shrink-0 items-center gap-2">
			<ReadRoleBadge />
			<MemberRowMenu disabled={disabled} onRemove={onRemove} />
		</div>
	</div>
);

export const ChatSharingPopoverContent: FC<ChatSharingPopoverContentProps> = ({
	chatId,
	organizationId,
	open,
}) => {
	const { user: currentUser } = useAuthenticated();
	const queryClient = useQueryClient();
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);

	const aclQuery = useQuery({
		...chatACL(chatId),
		enabled: open,
	});

	const {
		error: userRoleError,
		isPending: isUserRolePending,
		mutate: mutateUserRole,
		reset: resetUserRole,
	} = useMutation(setChatUserRole(queryClient));
	const {
		error: groupRoleError,
		isPending: isGroupRolePending,
		mutate: mutateGroupRole,
		reset: resetGroupRole,
	} = useMutation(setChatGroupRole(queryClient));

	const mutationError = userRoleError ?? groupRoleError;
	const isMutating = isUserRolePending || isGroupRolePending;

	const resetMutationErrors = () => {
		resetUserRole();
		resetGroupRole();
	};

	const acl = aclQuery.data;
	const users = (acl?.users ?? []).filter((user) => user.id !== currentUser.id);
	const groups = acl?.groups ?? [];
	const excludeFromAutocomplete = [...users, ...groups, currentUser];

	const handleAddMember = () => {
		if (!selectedOption || isMutating) {
			return;
		}

		if (!isGroup(selectedOption) && selectedOption.id === currentUser.id) {
			return;
		}

		resetMutationErrors();

		if (isGroup(selectedOption)) {
			mutateGroupRole(
				{
					chatId,
					groupId: selectedOption.id,
					role: "read",
				},
				{
					onSuccess: () => {
						setSelectedOption(null);
						toast.success("Group added to chat.");
					},
				},
			);
			return;
		}

		mutateUserRole(
			{
				chatId,
				userId: selectedOption.id,
				role: "read",
			},
			{
				onSuccess: () => {
					setSelectedOption(null);
					toast.success("Member added to chat.");
				},
			},
		);
	};

	const handleRemoveUser = (user: TypesGen.ChatUser) => {
		if (isMutating) {
			return;
		}

		resetMutationErrors();
		mutateUserRole(
			{ chatId, userId: user.id, role: "" },
			{ onSuccess: () => toast.success("Member removed from chat.") },
		);
	};

	const handleRemoveGroup = (group: TypesGen.ChatGroup) => {
		if (isMutating) {
			return;
		}

		resetMutationErrors();
		mutateGroupRole(
			{ chatId, groupId: group.id, role: "" },
			{ onSuccess: () => toast.success("Group removed from chat.") },
		);
	};

	const isEmpty = groups.length === 0 && users.length === 0;

	return (
		<PopoverContent
			align="end"
			className="w-[calc(100vw-2rem)] p-3 sm:w-[580px] sm:p-4"
		>
			<div className="flex items-center gap-2 mb-4">
				<h3 className="text-lg font-semibold m-0">Chat sharing</h3>
			</div>

			<div className="flex flex-col gap-4">
				{mutationError && <ErrorAlert error={mutationError} />}
				{aclQuery.error && <ErrorAlert error={aclQuery.error} />}

				{aclQuery.isLoading ? (
					<div role="status" className="flex flex-col items-center gap-4 py-8">
						<Spinner loading />
						<span>Loading chat sharing</span>
					</div>
				) : acl ? (
					<>
						<AddChatMemberForm
							isLoading={isMutating}
							disabled={!selectedOption}
							onSubmit={handleAddMember}
						>
							<UserOrGroupAutocomplete
								value={selectedOption}
								onChange={setSelectedOption}
								organizationId={organizationId}
								exclude={excludeFromAutocomplete}
								className="w-full sm:w-80"
							/>
						</AddChatMemberForm>

						{isEmpty ? (
							<div className="flex min-h-44 flex-col items-center justify-center px-6 py-6 text-center">
								<h4 className="m-0 text-sm font-medium text-content-secondary">
									No shared members or groups yet
								</h4>
								<p className="m-0 mt-2 text-sm text-content-secondary">
									Add a member or group using the controls above.
								</p>
							</div>
						) : (
							<div className="max-h-[min(60vh,24rem)] overflow-y-auto rounded-md border border-solid border-border sm:hidden">
								{groups.map((group) => (
									<MobileMemberRow
										key={group.id}
										disabled={isMutating}
										onRemove={() => handleRemoveGroup(group)}
									>
										<MemberIdentity kind="group" group={group} />
									</MobileMemberRow>
								))}
								{users.map((user) => (
									<MobileMemberRow
										key={user.id}
										disabled={isMutating}
										onRemove={() => handleRemoveUser(user)}
									>
										<MemberIdentity kind="user" user={user} />
									</MobileMemberRow>
								))}
							</div>
						)}

						{!isEmpty && (
							<div className="hidden sm:block">
								<Table
									aria-label="Shared chat members and groups"
									wrapperClassName="max-h-60 overflow-y-auto"
								>
									<TableHeader>
										<TableRow>
											<TableHead className="sticky top-0 z-10 w-[50%] bg-surface-primary py-2">
												Member
											</TableHead>
											<TableHead className="sticky top-0 z-10 w-[40%] bg-surface-primary py-2">
												Role
											</TableHead>
											<TableHead className="sticky top-0 z-10 w-[10%] bg-surface-primary py-2" />
										</TableRow>
									</TableHeader>
									<TableBody>
										{groups.map((group) => (
											<TableRow key={group.id}>
												<TableCell className="py-2 w-[50%]">
													<MemberIdentity kind="group" group={group} />
												</TableCell>
												<TableCell className="py-2 w-[40%]">
													<ReadRoleBadge />
												</TableCell>
												<TableCell className="py-2 w-[10%]">
													<MemberRowMenu
														disabled={isMutating}
														onRemove={() => handleRemoveGroup(group)}
													/>
												</TableCell>
											</TableRow>
										))}
										{users.map((user) => (
											<TableRow key={user.id}>
												<TableCell className="py-2 w-[50%]">
													<MemberIdentity kind="user" user={user} />
												</TableCell>
												<TableCell className="py-2 w-[40%]">
													<ReadRoleBadge />
												</TableCell>
												<TableCell className="py-2 w-[10%]">
													<MemberRowMenu
														disabled={isMutating}
														onRemove={() => handleRemoveUser(user)}
													/>
												</TableCell>
											</TableRow>
										))}
									</TableBody>
								</Table>
							</div>
						)}
					</>
				) : null}
			</div>
		</PopoverContent>
	);
};

export const ChatShareButton: FC<ChatShareButtonProps> = ({
	chatId,
	organizationId,
}) => {
	const [open, setOpen] = useState(false);
	const [contentGeneration, setContentGeneration] = useState(0);

	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen) {
			setContentGeneration((generation) => generation + 1);
		}

		setOpen(nextOpen);
	};

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<TopbarButton data-testid="chat-share-button">
					<Share2Icon />
					Share
				</TopbarButton>
			</PopoverTrigger>
			<ChatSharingPopoverContent
				key={contentGeneration}
				chatId={chatId}
				organizationId={organizationId}
				open={open}
			/>
		</Popover>
	);
};
