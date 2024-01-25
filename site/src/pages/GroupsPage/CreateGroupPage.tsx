import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { createGroup } from "api/queries/groups";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { pageTitle } from "utils/page";
import CreateGroupPageView from "./CreateGroupPageView";

export const CreateGroupPage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const organizationId = useOrganizationId();
  const createGroupMutation = useMutation(createGroup(queryClient));

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Group")}</title>
      </Helmet>
      <CreateGroupPageView
        onSubmit={async (data) => {
          const newGroup = await createGroupMutation.mutateAsync({
            organizationId,
            ...data,
          });
          navigate(`/groups/${newGroup.id}`);
        }}
        error={createGroupMutation.error}
        isLoading={createGroupMutation.isLoading}
      />
    </>
  );
};
export default CreateGroupPage;
