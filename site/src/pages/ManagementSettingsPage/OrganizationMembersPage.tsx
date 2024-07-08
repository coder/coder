import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { organizationMembers } from "api/queries/organizations";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Margins } from "components/Margins/Margins";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { me } from "api/queries/users";
import { AvatarIcon } from "components/Avatar/Avatar";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { AvatarData } from "components/AvatarData/AvatarData";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { Navigate, useParams } from "react-router-dom";

const OrganizationMembersPage: FC = () => {
  const queryClient = useQueryClient();
  const { organization } = useParams() as { organization: string };

  const membersQuery = useQuery(organizationMembers(organization));

  const { currentOrganizationId, organizations } = useOrganizationSettings();

  const error = membersQuery.error;

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
              <TableCell width="50%">Name</TableCell>
              <TableCell width="49%">Users</TableCell>
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
                <TableCell></TableCell>
                <TableCell></TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </div>
  );
};

export default OrganizationMembersPage;
