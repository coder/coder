import { useTheme } from "@emotion/react";
import { Tooltip } from "@mui/material";
import Table from "@mui/material/Table";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { AvatarData } from "components/AvatarData/AvatarData";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router-dom";
import {
  addOrganizationMember,
  organizationMembers,
  removeOrganizationMember,
} from "api/queries/organizations";
import { roles } from "api/queries/roles";
import type { OrganizationMemberWithUserData } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Pill } from "components/Pill/Pill";
import { useOrganizationSettings } from "./ManagementSettingsLayout";

const OrganizationMembersPage: FC = () => {
  const queryClient = useQueryClient();
  const { organization } = useParams() as { organization: string };

  const rolesQuery = useQuery(roles());
  const membersQuery = useQuery(organizationMembers(organization));
  const addMemberMutation = useMutation(
    addOrganizationMember(queryClient, organization),
  );
  const removeMemberMutation = useMutation(
    removeOrganizationMember(queryClient, organization),
  );

  const { currentOrganizationId, organizations } = useOrganizationSettings();

  const error = rolesQuery.error ?? membersQuery.error;

  const theme = useTheme();

  return (
    <div>
      <PageHeader>
        <PageHeaderTitle>Organization members</PageHeaderTitle>
      </PageHeader>

      {Boolean(error) && (
        <div css={{ marginBottom: 32 }}>
          <ErrorAlert error={error} />
        </div>
      )}

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="50%">User</TableCell>
              <TableCell width="49%">Roles</TableCell>
              <TableCell width="1%"></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {membersQuery.data?.map((member) => (
              <TableRow key={member.user_id}>
                <TableCell>
                  <AvatarData
                    avatar={
                      <UserAvatar
                        username={member.username}
                        avatarURL={member.avatar_url}
                      />
                    }
                    title={member.name}
                    subtitle={member.username}
                  />
                </TableCell>
                <TableCell>
                  {getMemberRoles(member).map((role) => (
                    <Pill
                      key={role.name}
                      css={{
                        backgroundColor: role.global
                          ? theme.roles.info.background
                          : theme.roles.inactive.background,
                        borderColor: role.global
                          ? theme.roles.info.outline
                          : theme.roles.inactive.outline,
                      }}
                    >
                      {role.name}
                      {role.global && (
                        <Tooltip title="This user has blah blah permissions for all organziations.">
                          <span>*</span>
                        </Tooltip>
                      )}
                    </Pill>
                  ))}
                </TableCell>
                <TableCell></TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </div>
  );
};

function getMemberRoles(member: OrganizationMemberWithUserData) {
  const roles = new Map<
    string,
    { name: string; global?: boolean; tooltip?: string }
  >();

  for (const role of member.global_roles) {
    roles.set(role.name, {
      name: role.display_name || role.name,
      global: true,
    });
  }
  for (const role of member.roles) {
    if (roles.has(role.name)) {
      continue;
    }
    roles.set(role.name, { name: role.display_name || role.name });
  }

  return [...roles.values()];
}

export default OrganizationMembersPage;
