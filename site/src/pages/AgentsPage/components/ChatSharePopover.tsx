import { EllipsisVertical, Share2Icon, UserPlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatACL,
	setChatGroupRole,
	setChatUserRole,
} from "#/api/queries/chats";
import type { ChatACLGroup, ChatACLUser } from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
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
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { isGroup } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";

interface ChatSharePopoverProps {
	chatId: string;
	organizationId: string;
}

export const ChatSharePopover: FC<ChatSharePopoverProps> = ({
	chatId,
	organizationId,
}) => {
	const queryClient = useQueryClient();
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);

	const aclQuery = useQuery(chatACL(chatId));
	const addUserMutation = useMutation(setChatUserRole(queryClient));
	const addGroupMutation = useMutation(setChatGroupRole(queryClient));
	const removeUserMutation = useMutation(setChatUserRole(queryClient));
	const removeGroupMutation = useMutation(setChatGroupRole(queryClient));

	const users = aclQuery.data?.users ?? [];
	const groups = aclQuery.data?.groups ?? [];

	const isAddPending = addUserMutation.isPending || addGroupMutation.isPending;
	const hasAddError = addUserMutation.isError || addGroupMutation.isError;
	const hasRemoveError =
		removeUserMutation.isError || removeGroupMutation.isError;

	const handleAdd = () => {
		if (!selectedOption) return;
		if (isGroup(selectedOption)) {
			addGroupMutation.mutate(
				{ chatId, groupId: selectedOption.id, role: "read" },
				{ onSuccess: () => setSelectedOption(null) },
			);
		} else {
			addUserMutation.mutate(
				{ chatId, userId: selectedOption.id, role: "read" },
				{ onSuccess: () => setSelectedOption(null) },
			);
		}
	};

	const handleRemoveUser = (user: ChatACLUser) => {
		removeUserMutation.mutate({ chatId, userId: user.id, role: "" });
	};

	const handleRemoveGroup = (group: ChatACLGroup) => {
		removeGroupMutation.mutate({ chatId, groupId: group.id, role: "" });
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

	const tableBody = (
		<TableBody>
			{aclQuery.isLoading ? (
				<TableLoader />
			) : users.length === 0 && groups.length === 0 ? (
				<TableRow>
					<TableCell colSpan={999}>
						<EmptyState
							message="Not shared with anyone yet"
							description="Add a member using the search above."
							isCompact
						/>
					</TableCell>
				</TableRow>
			) : (
				<>
					{groups.map((group) => (
						<TableRow key={`group-${group.id}`}>
							<TableCell className="py-2 w-[50%]">
								<AvatarData
									title={group.display_name || group.name}
									subtitle="Group"
									src={group.avatar_url}
								/>
							</TableCell>
							<TableCell className="py-2 w-[40%]">
								<span className="bg-surface-secondary rounded-md px-3 py-0.5 inline-block text-sm">
									Read only
								</span>
							</TableCell>
							<TableCell className="py-2 w-[10%]">
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button
											size="icon-lg"
											variant="subtle"
											aria-label="Open menu"
										>
											<EllipsisVertical aria-hidden="true" />
											<span className="sr-only">Open menu</span>
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent align="end">
										<DropdownMenuItem
											className="text-content-destructive focus:text-content-destructive"
											onClick={() => handleRemoveGroup(group)}
										>
											Remove
										</DropdownMenuItem>
									</DropdownMenuContent>
								</DropdownMenu>
							</TableCell>
						</TableRow>
					))}
					{users.map((user) => (
						<TableRow key={`user-${user.id}`}>
							<TableCell className="py-2 w-[50%]">
								<AvatarData
									title={user.username}
									subtitle={user.name}
									src={user.avatar_url}
								/>
							</TableCell>
							<TableCell className="py-2 w-[40%]">
								<span className="bg-surface-secondary rounded-md px-3 py-0.5 inline-block text-sm">
									Read only
								</span>
							</TableCell>
							<TableCell className="py-2 w-[10%]">
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button
											size="icon-lg"
											variant="subtle"
											aria-label="Open menu"
										>
											<EllipsisVertical aria-hidden="true" />
											<span className="sr-only">Open menu</span>
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent align="end">
										<DropdownMenuItem
											className="text-content-destructive focus:text-content-destructive"
											onClick={() => handleRemoveUser(user)}
										>
											Remove
										</DropdownMenuItem>
									</DropdownMenuContent>
								</DropdownMenu>
							</TableCell>
						</TableRow>
					))}
				</>
			)}
		</TableBody>
	);

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button
					size="icon"
					variant="subtle"
					className="h-7 w-7 text-content-secondary hover:text-content-primary"
					aria-label="Share chat"
				>
					<Share2Icon className="h-4 w-4" />
				</Button>
			</PopoverTrigger>
			<PopoverContent align="end" className="w-[580px] p-4">
				<h3 className="text-lg font-semibold m-0 mb-1">Share Chat</h3>
				<p className="mb-4 text-sm text-content-secondary">
					Add users or groups who can view this chat (read-only).
				</p>

				{hasAddError && (
					<p className="mb-2 text-xs text-content-destructive">
						Failed to add member. Please try again.
					</p>
				)}

				{hasRemoveError && (
					<p className="mb-2 text-xs text-content-destructive">
						Failed to remove member. Please try again.
					</p>
				)}

				<form
					action={handleAdd}
					className="flex flex-row items-center gap-2 mb-4"
				>
					<UserOrGroupAutocomplete
						organizationId={organizationId}
						value={selectedOption}
						exclude={[...users, ...groups]}
						onChange={(newValue) => setSelectedOption(newValue)}
					/>
					<Button disabled={!selectedOption || isAddPending} type="submit">
						<Spinner loading={isAddPending}>
							<UserPlusIcon className="size-icon-sm" />
						</Spinner>
						Add member
					</Button>
				</form>

				<div>
					<Table>{tableHeader}</Table>
					<div className="max-h-60 overflow-y-auto">
						<Table>{tableBody}</Table>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
};
