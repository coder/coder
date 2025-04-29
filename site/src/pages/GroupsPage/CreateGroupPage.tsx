import { createGroup } from "api/queries/groups";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import CreateGroupPageView from "./CreateGroupPageView";

const CreateGroupPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { organization } = useParams() as { organization: string };
	const createGroupMutation = useMutation(
		createGroup(queryClient, organization ?? "default"),
	);

	return (
		<>
			<Helmet>
				<title>{pageTitle("Create Group")}</title>
			</Helmet>
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
				isLoading={createGroupMutation.isLoading}
			/>
		</>
	);
};
export default CreateGroupPage;
