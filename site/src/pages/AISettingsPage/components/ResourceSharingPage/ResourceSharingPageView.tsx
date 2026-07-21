import {
	ArrowLeftIcon,
	EllipsisVerticalIcon,
	UserPlusIcon,
} from "lucide-react";
import { type FC, type FormEvent, useState } from "react";
import { Link } from "react-router";
import type {
	ChatModelConfigACL,
	ChatModelConfigGroup,
	ChatModelConfigUser,
	MCPServerConfigACL,
	MCPServerConfigGroup,
	MCPServerConfigUser,
} from "#/api/typesGenerated";
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
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
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
	type ResourceSharingCandidateValue,
	UserOrGroupAutocomplete,
} from "./UserOrGroupAutocomplete";

type ResourceACL = ChatModelConfigACL | MCPServerConfigACL;
type ResourceUser = ChatModelConfigUser | MCPServerConfigUser;
type ResourceGroup = ChatModelConfigGroup | MCPServerConfigGroup;

type ResourceSharingPageViewProps = {
	resourceName: string;
	resourceTypeLabel: string;
	backPath: string;
	search: string;
	organizationId: string;
	acl?: ResourceACL;
	isLoading: boolean;
	error: unknown;
	mutationError: unknown;
	canShare: boolean;
	isMutating: boolean;
	onAddUser: (userId: string) => Promise<void>;
	onAddGroup: (groupId: string) => Promise<void>;
	onRemoveUser: (userId: string) => Promise<void>;
	onRemoveGroup: (groupId: string) => Promise<void>;
};

const ReadRoleBadge: FC = () => (
	<span className="inline-block shrink-0 rounded-md bg-surface-secondary px-2 py-0.5 text-xs leading-5">
		Read
	</span>
);

const MemberIdentity: FC<
	{ kind: "user"; user: ResourceUser } | { kind: "group"; group: ResourceGroup }
> = (props) => {
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
	return (
		<AvatarData
			title={props.user.username}
			subtitle={props.user.name}
			src={props.user.avatar_url}
		/>
	);
};

const RemoveMenu: FC<{
	disabled: boolean;
	label: string;
	onRemove: () => void;
}> = ({ disabled, label, onRemove }) => (
	<DropdownMenu>
		<DropdownMenuTrigger asChild>
			<Button
				variant="subtle"
				size="icon"
				disabled={disabled}
				aria-label={`Manage ${label}`}
			>
				<EllipsisVerticalIcon />
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

export const ResourceSharingPageView: FC<ResourceSharingPageViewProps> = ({
	resourceName,
	resourceTypeLabel,
	backPath,
	search,
	organizationId,
	acl,
	isLoading,
	error,
	mutationError,
	canShare,
	isMutating,
	onAddUser,
	onAddGroup,
	onRemoveUser,
	onRemoveGroup,
}) => {
	const [selectedCandidate, setSelectedCandidate] =
		useState<ResourceSharingCandidateValue>(null);
	const users = acl?.users ?? [];
	const groups = acl?.groups ?? [];
	const isEmpty = users.length === 0 && groups.length === 0;
	const excludeIds = [...users, ...groups].map((entry) => entry.id);

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (!selectedCandidate || isMutating) {
			return;
		}
		try {
			if (isGroup(selectedCandidate)) {
				await onAddGroup(selectedCandidate.id);
			} else {
				await onAddUser(selectedCandidate.id);
			}
			setSelectedCandidate(null);
		} catch {
			// The page displays the mutation error and keeps the selection for retry.
		}
	};

	return (
		<div>
			<Link
				to={{ pathname: backPath, search }}
				className="mb-4 inline-flex -ml-3 no-underline"
			>
				<Button variant="subtle" type="button">
					<ArrowLeftIcon />
					Back to {resourceTypeLabel}
				</Button>
			</Link>
			<SettingsHeader>
				<SettingsHeaderTitle>Share {resourceName}</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Grant users and groups read access to this {resourceTypeLabel}.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<div className="flex flex-col gap-4">
				{Boolean(error) && <ErrorAlert error={error} />}
				{Boolean(mutationError) && <ErrorAlert error={mutationError} />}
				{isLoading ? (
					<div role="status" className="flex flex-col items-center gap-4 py-12">
						<Spinner loading />
						<span>Loading sharing settings</span>
					</div>
				) : acl ? (
					<>
						{canShare ? (
							<form onSubmit={handleSubmit}>
								<div className="flex flex-col gap-2 sm:flex-row sm:items-center">
									<div className="min-w-0 flex-1">
										<UserOrGroupAutocomplete
											value={selectedCandidate}
											onChange={setSelectedCandidate}
											organizationId={organizationId}
											excludeIds={excludeIds}
										/>
									</div>
									<Button
										type="submit"
										disabled={!selectedCandidate || isMutating}
									>
										<Spinner loading={isMutating}>
											<UserPlusIcon />
										</Spinner>
										Add member
									</Button>
								</div>
							</form>
						) : (
							<p className="m-0 rounded-md border border-solid border-border bg-surface-secondary p-4 text-sm text-content-secondary">
								You can view sharing settings, but you do not have permission to
								change them.
							</p>
						)}

						{isEmpty ? (
							<div className="flex min-h-48 flex-col items-center justify-center rounded-md border border-solid border-border px-6 py-8 text-center">
								<h3 className="m-0 text-base font-medium">
									No users or groups have access
								</h3>
								<p className="m-0 mt-2 text-sm text-content-secondary">
									{canShare
										? "Add a user or group using the controls above."
										: "A user with sharing permission can grant access."}
								</p>
							</div>
						) : (
							<Table
								aria-label={`Users and groups with access to ${resourceName}`}
							>
								<TableHeader>
									<TableRow>
										<TableHead>Member</TableHead>
										<TableHead className="w-40">Role</TableHead>
										{canShare && <TableHead className="w-16" />}
									</TableRow>
								</TableHeader>
								<TableBody>
									{groups.map((group) => (
										<TableRow key={group.id}>
											<TableCell>
												<MemberIdentity kind="group" group={group} />
											</TableCell>
											<TableCell>
												<ReadRoleBadge />
											</TableCell>
											{canShare && (
												<TableCell>
													<RemoveMenu
														disabled={isMutating}
														label={group.display_name || group.name}
														onRemove={() => {
															void onRemoveGroup(group.id).catch(
																() => undefined,
															);
														}}
													/>
												</TableCell>
											)}
										</TableRow>
									))}
									{users.map((user) => (
										<TableRow key={user.id}>
											<TableCell>
												<MemberIdentity kind="user" user={user} />
											</TableCell>
											<TableCell>
												<ReadRoleBadge />
											</TableCell>
											{canShare && (
												<TableCell>
													<RemoveMenu
														disabled={isMutating}
														label={user.username}
														onRemove={() => {
															void onRemoveUser(user.id).catch(() => undefined);
														}}
													/>
												</TableCell>
											)}
										</TableRow>
									))}
								</TableBody>
							</Table>
						)}
					</>
				) : null}
			</div>
		</div>
	);
};
