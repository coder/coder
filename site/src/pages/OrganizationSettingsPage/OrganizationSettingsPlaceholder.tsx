import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import {
  createOrganization,
  deleteOrganization,
} from "api/queries/organizations";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Margins } from "components/Margins/Margins";
import { useOrganizationSettings } from "./OrganizationSettingsLayout";

const OrganizationSettingsPage: FC = () => {
  const queryClient = useQueryClient();
  const addOrganizationMutation = useMutation(createOrganization(queryClient));
  const deleteOrganizationMutation = useMutation(
    deleteOrganization(queryClient),
  );

  const { currentOrganizationId, organizations } = useOrganizationSettings();

  const org = organizations.find((org) => org.id === currentOrganizationId)!;

  const error =
    addOrganizationMutation.error ?? deleteOrganizationMutation.error;

  return (
    <Margins css={{ marginTop: 48, marginBottom: 48 }}>
      {Boolean(error) && <ErrorAlert error={error} />}

      <h1>Organization settings</h1>

      <p>Name: {org.name}</p>
      <p>Display name: {org.display_name}</p>
    </Margins>
  );
};

export default OrganizationSettingsPage;
