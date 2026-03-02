import { getErrorDetail, getErrorMessage } from "api/errors";
import { deleteGroup, group, groupPermissions } from "api/queries/groups";
import type { Group } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Loader } from "components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { TrashIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Outlet, useLocation, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { pageTitle } from "utils/page";

export type GroupPageOutletContext = {
	group: Group;
	permissions: { canUpdateGroup: boolean };
	organization: string;
	groupQuery: ReturnType<typeof useQuery>;
};

const GroupPage: FC = () => {
	const { organization = "default", groupName } = useParams() as {
		organization?: string;
		groupName: string;
	};
	const location = useLocation();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const groupQuery = useQuery(group(organization, groupName));
	const groupData = groupQuery.data;
	const { data: permissions } = useQuery({
		...groupPermissions(groupData?.id ?? ""),
		enabled: !!groupData,
	});
	const deleteGroupMutation = useMutation(
		deleteGroup(queryClient, organization),
	);
	const [isDeletingGroup, setIsDeletingGroup] = useState(false);
	const isLoading = groupQuery.isLoading || !groupData || !permissions;
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;

	const title = (
		<title>
			{pageTitle((groupData?.display_name || groupData?.name) ?? "Loading...")}
		</title>
	);

	if (groupQuery.error) {
		return <ErrorAlert error={groupQuery.error} />;
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

			<div className="flex align-baseline justify-between w-full">
				<SettingsHeader>
					<SettingsHeaderTitle>
						{groupData.display_name || groupData.name || "Unknown Group"}
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Manage members for this group.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{canUpdateGroup && (
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
				)}
			</div>
			<div className="flex flex-col gap-10 w-full">
				{canUpdateGroup && (
					<Tabs active={activeTab}>
						<TabsList className="w-full justify-start">
							<TabLink to="." value="members">
								Group members
							</TabLink>
							<TabLink to="settings" value="settings">
								Group settings
							</TabLink>
						</TabsList>
					</Tabs>
				)}

				<Outlet
					context={
						{
							group: groupData,
							permissions: { canUpdateGroup },
							organization,
							groupQuery,
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

export default GroupPage;
