import { EllipsisVerticalIcon, Share2Icon } from "lucide-react";
import { type FC, useState } from "react";
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
import { EmptyState } from "#/components/EmptyState/EmptyState";
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
import { AddWorkspaceMemberForm } from "#/modules/workspaces/WorkspaceSharingForm/WorkspaceSharingForm";

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
	<span className="bg-surface-secondary rounded-md px-3 py-0.5 inline-block">
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
				<span className="sr-only">Open menu</span>
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

	const userRoleMutation = useMutation(setChatUserRole(queryClient));
	const groupRoleMutation = useMutation(setChatGroupRole(queryClient));

	const mutationError = userRoleMutation.error ?? groupRoleMutation.error;
	const isMutating = userRoleMutation.isPending || groupRoleMutation.isPending;

	const acl = aclQuery.data;
	const users = (acl?.users ?? []).filter((user) => user.id !== currentUser.id);
	const groups = acl?.groups ?? [];
	const excludeFromAutocomplete = [...users, ...groups, currentUser];

	const handleAddMember = () => {
		if (!selectedOption) {
			return;
		}

		const onSuccess = () => {
			setSelectedOption(null);
			toast.success("Member added to chat.");
		};

		if (!isGroup(selectedOption) && selectedOption.id === currentUser.id) {
			return;
		}

		if (isGroup(selectedOption)) {
			groupRoleMutation.mutate(
				{
					chatId,
					groupId: selectedOption.id,
					role: "read",
				},
				{ onSuccess },
			);
			return;
		}

		userRoleMutation.mutate(
			{
				chatId,
				userId: selectedOption.id,
				role: "read",
			},
			{ onSuccess },
		);
	};

	const handleRemoveUser = (user: TypesGen.ChatUser) => {
		userRoleMutation.mutate(
			{ chatId, userId: user.id, role: "" },
			{ onSuccess: () => toast.success("Member removed from chat.") },
		);
	};

	const handleRemoveGroup = (group: TypesGen.ChatGroup) => {
		groupRoleMutation.mutate(
			{ chatId, groupId: group.id, role: "" },
			{ onSuccess: () => toast.success("Group removed from chat.") },
		);
	};

	const tableHeader = (
		<TableHeader>
			<TableRow>
				<TableHead className="w-[50%] py-2">Member</TableHead>
				<TableHead className="w-[40%] py-2">Role</TableHead>
				<TableHead className="w-[10%] py-2" />
			</TableRow>
		</TableHeader>
	);

	const isEmpty = groups.length === 0 && users.length === 0;

	const tableBody = (
		<TableBody>
			{isEmpty ? (
				<TableRow>
					<TableCell colSpan={999}>
						<EmptyState
							message="No shared members or groups yet"
							description="Add a member or group using the controls above."
							isCompact
						/>
					</TableCell>
				</TableRow>
			) : (
				<>
					{groups.map((group) => (
						<TableRow key={group.id}>
							<TableCell className="py-2 w-[50%]">
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
								<AvatarData
									title={user.username}
									subtitle={user.name}
									src={user.avatar_url}
								/>
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
				</>
			)}
		</TableBody>
	);

	return (
		<PopoverContent align="end" className="w-[580px] p-4">
			<div className="flex items-center gap-2 mb-4">
				<h3 className="text-lg font-semibold m-0">Chat Sharing</h3>
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
						<AddWorkspaceMemberForm
							isLoading={isMutating}
							disabled={!selectedOption}
							onSubmit={handleAddMember}
						>
							<UserOrGroupAutocomplete
								value={selectedOption}
								onChange={setSelectedOption}
								organizationId={organizationId}
								exclude={excludeFromAutocomplete}
							/>
						</AddWorkspaceMemberForm>

						<div>
							<Table>{tableHeader}</Table>
							<div className="max-h-60 overflow-y-auto">
								<Table>{tableBody}</Table>
							</div>
						</div>
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

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<TopbarButton data-testid="chat-share-button">
					<Share2Icon />
					Share
				</TopbarButton>
			</PopoverTrigger>
			<ChatSharingPopoverContent
				chatId={chatId}
				organizationId={organizationId}
				open={open}
			/>
		</Popover>
	);
};
