import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate } from "react-router-dom";
import { pageTitle } from "utils/page";
import CreateGroupPageView from "./CreateGroupPageView";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createGroup } from "api/queries/groups";

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
        formErrors={createGroupMutation.error}
        isLoading={createGroupMutation.isLoading}
      />
    </>
  );
};
export default CreateGroupPage;
