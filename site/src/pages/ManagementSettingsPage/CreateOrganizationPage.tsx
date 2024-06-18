import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { createOrganization } from "api/queries/organizations";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const CreateOrganizationPage: FC = () => {
  const navigate = useNavigate();

  const queryClient = useQueryClient();
  const createOrganizationMutation = useMutation(
    createOrganization(queryClient),
  );

  const error = createOrganizationMutation.error;

  return (
    <Stack>
      {Boolean(error) && <ErrorAlert error={error} />}

      <CreateOrganizationPageView
        error={error}
        onSubmit={async (values) => {
          await createOrganizationMutation.mutateAsync(values);
          displaySuccess("Organization settings updated.");
          navigate(`/organizations/${values.name}`);
        }}
      />
    </Stack>
  );
};

export default CreateOrganizationPage;
