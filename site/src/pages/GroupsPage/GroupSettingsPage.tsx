import { getErrorMessage } from "api/errors";
import { group, patchGroup } from "api/queries/groups";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
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

	const helmet = (
		<Helmet>
			<title>{pageTitle("Settings Group")}</title>
		</Helmet>
	);

	if (groupQuery.error) {
		return <ErrorAlert error={groupQuery.error} />;
	}

	if (groupQuery.isLoading || !groupQuery.data) {
		return (
			<>
				{helmet}
				<Loader />
			</>
		);
	}
	const groupId = groupQuery.data.id;

	return (
		<>
			{helmet}

			<GroupSettingsPageView
				onCancel={navigateToGroup}
				onSubmit={async (data) => {
					try {
						await patchGroupMutation.mutateAsync({
							groupId,
							...data,
							add_users: [],
							remove_users: [],
						});
						navigate(`../${data.name}`);
					} catch (error) {
						displayError(getErrorMessage(error, "Failed to update group"));
					}
				}}
				group={groupQuery.data}
				formErrors={groupQuery.error}
				isLoading={groupQuery.isLoading}
				isUpdating={patchGroupMutation.isLoading}
			/>
		</>
	);
};
export default GroupSettingsPage;
