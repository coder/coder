import { TrashIcon, UserPlusIcon } from "lucide-react";
import { type ComponentProps, type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	Outlet,
	useLocation,
	useNavigate,
	useParams,
	useSearchParams,
} from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	addMembers,
	deleteGroup,
	group,
	groupMembers,
	groupPermissions,
} from "#/api/queries/groups";
import type {
	Group,
	OrganizationMemberWithUserData,
	ReducedUser,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { useFilter } from "#/components/Filter/Filter";
import type { UsersFilter } from "#/components/Filter/UsersFilter";
import { Loader } from "#/components/Loader/Loader";
import { MultiMemberSelect } from "#/components/MultiUserSelect/MultiUserSelect";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { LinkTabs, LinkTabsList, TabLink } from "#/components/Tabs/Tabs";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { isEveryoneGroup } from "#/modules/groups";
import { pageTitle } from "#/utils/page";
import { AIBudgetPeriod } from "./AIBudgetPeriod";

export type GroupPageOutletContext = {
	group: Group;
	members: readonly ReducedUser[];
	permissions: { canUpdateGroup: boolean };
	organization: string;
	groupQuery: ReturnType<typeof useQuery>;
	membersQuery: PaginationResult;
	filterProps: ComponentProps<typeof UsersFilter>;
};

const GroupPage: FC = () => {
	const { organization = "default", groupName } = useParams() as {
		organization?: string;
		groupName: string;
	};
	const location = useLocation();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const [searchParams, setSearchParams] = useSearchParams();
	const groupQuery = useQuery(
		group(organization, groupName, { exclude_members: true }),
	);
	const membersQuery = usePaginatedQuery(
		groupMembers(organization, groupName, searchParams),
	);
	const useFilterResult = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: membersQuery.goToFirstPage,
	});

	const groupData = groupQuery.data;
	const { data: permissions } = useQuery({
		...groupPermissions(groupData?.id ?? ""),
		enabled: Boolean(groupData),
	});
	const deleteGroupMutation = useMutation(
		deleteGroup(queryClient, organization),
	);
	const addMembersMutation = useMutation(addMembers(queryClient, organization));
	const [isDeletingGroup, setIsDeletingGroup] = useState(false);
	const isLoading =
		groupQuery.isLoading ||
		!groupData ||
		!permissions ||
		membersQuery.isLoading ||
		!membersQuery.data;
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;

	const title = (
		<title>
			{pageTitle((groupData?.display_name || groupData?.name) ?? "Loading...")}
		</title>
	);

	const error = groupQuery.error || membersQuery.error;
	if (error) {
		return <ErrorAlert error={error} />;
	}

	if (isLoading) {
		return (
			<>
				{title}
				<Loader />
			</>
		);
	}

	const groupId = groupData.id;
	const activeTab = location.pathname.endsWith("/settings")
		? "settings"
		: "members";

	return (
		<>
			{title}

			<SettingsHeader
				actions={
					canUpdateGroup && (
						<div className="flex items-center gap-2">
							{!isEveryoneGroup(groupData) && (
								<AddUsersDialog
									organizationId={groupData.organization_id}
									onSubmit={async (users) => {
										await addMembersMutation.mutateAsync({
											groupId: groupData.id,
											userIds: users.map((u) => u.user_id),
										});
									}}
								/>
							)}
							<Button
								variant="destructive"
								disabled={groupData.id === groupData.organization_id}
								onClick={() => {
									setIsDeletingGroup(true);
								}}
							>
								<TrashIcon />
								Delete&hellip;
							</Button>
						</div>
					)
				}
			>
				<AvatarData
					avatar={
						<Avatar
							src={groupData.avatar_url}
							fallback={groupData.display_name || groupData.name}
							size="lg"
						/>
					}
					title={
						<SettingsHeaderTitle>
							{groupData.display_name || groupData.name || "Unknown Group"}
						</SettingsHeaderTitle>
					}
				/>
				<SettingsHeaderDescription>
					Manage members for this group.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<div className="flex flex-col gap-10 w-full">
				{canUpdateGroup && (
					<LinkTabs
						active={activeTab}
						className="flex items-baseline justify-between"
					>
						<LinkTabsList className="justify-start">
							<TabLink to="." value="members">
								Group members
							</TabLink>
							<TabLink to="settings" value="settings">
								Group settings
							</TabLink>
						</LinkTabsList>
						{activeTab === "members" && <AIBudgetPeriod />}
					</LinkTabs>
				)}

				<Outlet
					context={
						{
							group: groupData,
							members: membersQuery.data?.users || [],
							permissions: { canUpdateGroup },
							organization,
							groupQuery,
							membersQuery,
							filterProps: {
								filter: useFilterResult,
							},
						} satisfies GroupPageOutletContext
					}
				/>
			</div>

			{groupQuery.data && (
				<DeleteDialog
					isOpen={isDeletingGroup}
					confirmLoading={deleteGroupMutation.isPending}
					name={groupQuery.data.name}
					entity="group"
					onConfirm={async () => {
						try {
							await deleteGroupMutation.mutateAsync({
								groupId,
								groupName: groupData.name,
							});
							toast.success(
								`Group "${groupQuery.data.name}" deleted successfully.`,
							);
							navigate("..");
						} catch (error) {
							toast.error(
								getErrorMessage(
									error,
									`Failed to delete group "${groupQuery.data.name}".`,
								),
								{
									description: getErrorDetail(error),
								},
							);
						}
					}}
					onCancel={() => {
						setIsDeletingGroup(false);
					}}
				/>
			)}
		</>
	);
};

interface AddUsersDialogProps {
	onSubmit: (users: OrganizationMemberWithUserData[]) => Promise<void>;
	organizationId: string;
}

const AddUsersDialog: FC<AddUsersDialogProps> = ({
	onSubmit,
	organizationId,
}) => {
	const [addUserDialogOpen, setAddUserDialogOpen] = useState(false);
	const [submitting, setSubmitting] = useState(false);
	const [filter, setFilter] = useState("");
	const [selected, setSelected] = useState<OrganizationMemberWithUserData[]>(
		[],
	);
	const closeDialog = () => {
		setAddUserDialogOpen(false);
		setFilter("");
		setSelected([]);
	};

	return (
		<>
			<Button onClick={() => setAddUserDialogOpen(true)}>
				<UserPlusIcon />
				Add users
			</Button>
			<Dialog
				open={addUserDialogOpen}
				onOpenChange={(open) => {
					if (!open) {
						closeDialog();
					}
				}}
			>
				<DialogContent
					data-testid="dialog"
					className="max-w-md gap-4 border-border-default bg-surface-primary p-8 text-content-primary"
				>
					<DialogTitle className="font-semibold text-content-primary">
						Add user(s)
					</DialogTitle>
					<MultiMemberSelect
						organizationId={organizationId}
						filter={filter}
						setFilter={setFilter}
						onChange={(user, checked) => {
							if (checked) {
								setSelected([...selected, user]);
							} else {
								setSelected(selected.filter((s) => s.user_id !== user.user_id));
							}
						}}
						selected={selected}
					/>
					<DialogFooter className="mt-4 flex-row justify-end gap-3">
						<Button
							variant="outline"
							onClick={closeDialog}
							disabled={submitting}
						>
							Cancel
						</Button>
						<Button
							disabled={submitting || selected.length === 0}
							onClick={async () => {
								try {
									setSubmitting(true);
									await onSubmit(selected);
									closeDialog();
								} catch (error) {
									toast.error(
										getErrorMessage(error, "Failed to add members."),
										{
											description: getErrorDetail(error),
										},
									);
								} finally {
									setSubmitting(false);
								}
							}}
						>
							<Spinner loading={submitting} />
							Add users
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
};

export default GroupPage;
