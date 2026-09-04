import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { createGroup } from "#/api/queries/groups";
import { pageTitle } from "#/utils/page";
import { CreateGroupPageView } from "./CreateGroupPageView";
import { useGroupsSettings } from "./GroupsPageProvider";

const CreateGroupPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { showOrganizations } = useGroupsSettings();
	const { organization } = useParams() as { organization: string };
	const createGroupMutation = useMutation(
		createGroup(queryClient, organization ?? "default"),
	);

	return (
		<>
			<title>{pageTitle("New group")}</title>

			<CreateGroupPageView
				onSubmit={async (data) => {
					const newGroup = await createGroupMutation.mutateAsync(data);
					navigate(
						organization
							? `/organizations/${organization}/groups/${newGroup.name}`
							: `/deployment/groups/${newGroup.name}`,
					);
				}}
				onCancel={() => {
					navigate("..");
				}}
				error={createGroupMutation.error}
				isLoading={createGroupMutation.isPending}
				showOrganizations={showOrganizations}
			/>
		</>
	);
};
export default CreateGroupPage;
