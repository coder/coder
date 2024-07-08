import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { organizationMembers } from "api/queries/organizations";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Margins } from "components/Margins/Margins";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { me } from "api/queries/users";

const OrganizationMembersPage: FC = () => {
  const queryClient = useQueryClient();

  const membersQuery = useQuery(organizationMembers("default"));

  const { currentOrganizationId, organizations } = useOrganizationSettings();

  const org = organizations.find((org) => org.id === currentOrganizationId)!;

  const error = membersQuery.error;

  return (
    <Margins css={{ marginTop: 48, marginBottom: 48 }}>
      {Boolean(error) && <ErrorAlert error={error} />}

      <h1>Organization settings</h1>

      <p>Name: {org.name}</p>
      <p>Display name: {org.display_name}</p>
      <p>Members: {membersQuery.data?.map((member) => member.username)}</p>

      {membersQuery.data && (
        <ul>
          {membersQuery.data.map((member) => (
            <li>
              {member.name} {member.avatar_url}
            </li>
          ))}
        </ul>
      )}
    </Margins>
  );
};

export default OrganizationMembersPage;
