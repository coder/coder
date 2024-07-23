import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { createGroup } from "api/queries/groups";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import CreateGroupPageView from "./CreateGroupPageView";

export const CreateGroupPage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const createGroupMutation = useMutation(createGroup(queryClient, "default"));
  const { experiments } = useDashboard();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Group")}</title>
      </Helmet>
      <CreateGroupPageView
        onSubmit={async (data) => {
          const newGroup = await createGroupMutation.mutateAsync(data);

          let groupURL = `/groups/${newGroup.name}`;
          if (experiments.includes("multi-organization")) {
            groupURL = `/organizations/${newGroup.organization_id}/groups/${newGroup.name}`;
          }

          navigate(groupURL);
        }}
        error={createGroupMutation.error}
        isLoading={createGroupMutation.isLoading}
      />
    </>
  );
};
export default CreateGroupPage;
