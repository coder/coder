import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import { group, patchGroup } from "api/queries/groups";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { pageTitle } from "utils/page";
import SettingsGroupPageView from "./SettingsGroupPageView";

export const SettingsGroupPage: FC = () => {
  const { groupName } = useParams() as { groupName: string };
  const queryClient = useQueryClient();
  const groupQuery = useQuery(group(groupName));
  const patchGroupMutation = useMutation(patchGroup(queryClient));
  const navigate = useNavigate();
  const groupData = groupQuery.data;
  const { isLoading, error} = groupQuery

  const navigateToGroup = () => {
    navigate(`/groups/${groupName}`);
  };

  const helmet = (
    <Helmet>
      <title>{pageTitle("Settings Group")}</title>
    </Helmet>
  );

  if (error) {
    return <ErrorAlert error={error} />;
  }

  if (isLoading || !groupData) {
    return (
      <>
        {helmet}
        <Loader />
      </>
    );
  }
  const groupId = groupData.id;

  return (
    <>
      {helmet}

      <SettingsGroupPageView
        onCancel={navigateToGroup}
        onSubmit={async (data) => {
          try {
            await patchGroupMutation.mutateAsync({
              groupId,
              ...data,
              add_users: [],
              remove_users: [],
            });
            navigate(`/groups/${data.name}`, { replace: true });
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
export default SettingsGroupPage;
