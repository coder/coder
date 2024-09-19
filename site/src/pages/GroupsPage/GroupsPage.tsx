import { getErrorMessage } from "api/errors";
import { groupsByOrganization } from "api/queries/groups";
import { displayError } from "components/GlobalSnackbar/utils";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import GroupsPageView from "./GroupsPageView";

export const GroupsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
	const groupsQuery = useQuery(groupsByOrganization("default"));

	useEffect(() => {
		if (groupsQuery.error) {
			displayError(
				getErrorMessage(groupsQuery.error, "Unable to load groups."),
			);
		}
	}, [groupsQuery.error]);

	return (
		<>
			<Helmet>
				<title>{pageTitle("Groups")}</title>
			</Helmet>

			<GroupsPageView
				groups={groupsQuery.data}
				canCreateGroup={permissions.createGroup}
				isTemplateRBACEnabled={isTemplateRBACEnabled}
			/>
		</>
	);
};

export default GroupsPage;
