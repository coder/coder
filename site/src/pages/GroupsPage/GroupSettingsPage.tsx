import { getErrorDetail, getErrorMessage } from "api/errors";
import { group, patchGroup } from "api/queries/groups";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { pageTitle } from "utils/page";
import GroupSettingsPageView from "./GroupSettingsPageView";

const GroupSettingsPage: FC = () => {
	const { organization = "default", groupName } = useParams() as {
		organization?: string;
		groupName: string;
	};
	const queryClient = useQueryClient();
	const groupQuery = useQuery(group(organization, groupName));
	const patchGroupMutation = useMutation(patchGroup(queryClient));
	const navigate = useNavigate();

	const navigateToGroup = () => {
		navigate(`/organizations/${organization}/groups/${groupName}`);
	};

	const title = <title>{pageTitle("Settings Group")}</title>;

	if (groupQuery.error) {
		return <ErrorAlert error={groupQuery.error} />;
	}

	if (groupQuery.isLoading || !groupQuery.data) {
		return (
			<>
				{title}
				<Loader />
			</>
		);
	}
	const groupId = groupQuery.data.id;

	return (
		<>
			{title}

			<GroupSettingsPageView
				onCancel={navigateToGroup}
				onSubmit={async (data) => {
					await patchGroupMutation.mutateAsync(
						{
							groupId,
							...data,
							add_users: [],
							remove_users: [],
						},
						{
							onSuccess: () => {
								navigate(`../${data.name}`);
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
				group={groupQuery.data}
				formErrors={groupQuery.error}
				isLoading={groupQuery.isLoading}
				isUpdating={patchGroupMutation.isPending}
			/>
		</>
	);
};
export default GroupSettingsPage;
