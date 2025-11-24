import { createGroup } from "api/queries/groups";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { pageTitle } from "utils/page";
import { CreateGroupPageView } from "./CreateGroupPageView";

const CreateGroupPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { organization } = useParams() as { organization: string };
	const createGroupMutation = useMutation(
		createGroup(queryClient, organization ?? "default"),
	);

	return (
		<>
			<title>{pageTitle("Create Group")}</title>

			<CreateGroupPageView
				onSubmit={async (data) => {
					const newGroup = await createGroupMutation.mutateAsync(data);
					navigate(
						organization
							? `/organizations/${organization}/groups/${newGroup.name}`
							: `/deployment/groups/${newGroup.name}`,
					);
				}}
				error={createGroupMutation.error}
				isLoading={createGroupMutation.isPending}
			/>
		</>
	);
};
export default CreateGroupPage;
