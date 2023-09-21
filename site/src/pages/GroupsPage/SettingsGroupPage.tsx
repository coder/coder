import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import SettingsGroupPageView from "./SettingsGroupPageView";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { group, patchGroup } from "api/queries/groups";
import { displayError } from "components/GlobalSnackbar/utils";
import { getErrorMessage } from "api/errors";

export const SettingsGroupPage: FC = () => {
  const { groupId } = useParams() as { groupId: string };
  const queryClient = useQueryClient();
  const groupQuery = useQuery(group(groupId));
  const patchGroupMutation = useMutation(patchGroup(queryClient));
  const navigate = useNavigate();

  const navigateToGroup = () => {
    navigate(`/groups/${groupId}`);
  };

  return (
    <>
      <Helmet>
        <title>{pageTitle("Settings Group")}</title>
      </Helmet>

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
            navigateToGroup();
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
