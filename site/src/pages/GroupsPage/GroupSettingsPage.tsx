import { getErrorDetail, getErrorMessage } from "api/errors";
import { patchGroup } from "api/queries/groups";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useOutletContext, useParams } from "react-router";
import { toast } from "sonner";
import type { GroupPageOutletContext } from "./GroupPage";
import GroupSettingsPageView from "./GroupSettingsPageView";

const GroupSettingsPage: FC = () => {
	const { organization = "default", groupName } = useParams() as {
		organization?: string;
		groupName: string;
	};
	const { group: groupData } = useOutletContext<GroupPageOutletContext>();
	const queryClient = useQueryClient();
	const patchGroupMutation = useMutation(patchGroup(queryClient, organization));
	const navigate = useNavigate();

	return (
		<GroupSettingsPageView
			onCancel={() => navigate("..")}
			onSubmit={async (data) => {
				await patchGroupMutation.mutateAsync(
					{
						groupId: groupData.id,
						...data,
						add_users: [],
						remove_users: [],
					},
					{
						onSuccess: () => {
							navigate(`/organizations/${organization}/groups/${data.name}`);
						},
						onError: (error) => {
							toast.error(
								getErrorMessage(
									error,
									`Failed to update group "${groupName}".`,
								),
								{
									description: getErrorDetail(error),
								},
							);
						},
					},
				);
			}}
			group={groupData}
			formErrors={undefined}
			isLoading={false}
			isUpdating={patchGroupMutation.isPending}
		/>
	);
};

export default GroupSettingsPage;
