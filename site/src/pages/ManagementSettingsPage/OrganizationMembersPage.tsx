import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import PersonAdd from "@mui/icons-material/PersonAdd";
import LoadingButton from "@mui/lab/LoadingButton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Tooltip from "@mui/material/Tooltip";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import {
  addOrganizationMember,
  organizationMembers,
  removeOrganizationMember,
} from "api/queries/organizations";
import type { OrganizationMemberWithUserData, User } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { AvatarData } from "components/AvatarData/AvatarData";
import { displayError } from "components/GlobalSnackbar/utils";
import {
  MoreMenu,
  MoreMenuTrigger,
  MoreMenuContent,
  MoreMenuItem,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { UserAvatar } from "components/UserAvatar/UserAvatar";

const OrganizationMembersPage: FC = () => {
  const queryClient = useQueryClient();
  const theme = useTheme();
  const { organization } = useParams() as { organization: string };

  const membersQuery = useQuery(organizationMembers(organization));
  const addMemberMutation = useMutation(
    addOrganizationMember(queryClient, organization),
  );
  const removeMemberMutation = useMutation(
    removeOrganizationMember(queryClient, organization),
  );

  const error =
    membersQuery.error ?? addMemberMutation.error ?? removeMemberMutation.error;

  return (
    <div>
      <PageHeader>
        <PageHeaderTitle>Organization members</PageHeaderTitle>
      </PageHeader>

      <Stack>
        {Boolean(error) && <ErrorAlert error={error} />}

        <AddGroupMember
          isLoading={addMemberMutation.isLoading}
          onSubmit={async (user) => {
            await addMemberMutation.mutateAsync(user.id);
            void membersQuery.refetch();
          }}
        />

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
                        {role.global ? (
                          <Tooltip title="This user has this role for all organizations.">
                            <span>{role.name}*</span>
                          </Tooltip>
                        ) : (
                          role.name
                        )}
                      </Pill>
                    ))}
                  </TableCell>
                  <TableCell>
                    <MoreMenu>
                      <MoreMenuTrigger>
                        <ThreeDotsButton />
                      </MoreMenuTrigger>
                      <MoreMenuContent>
                        <MoreMenuItem
                          danger
                          onClick={async () => {
                            await removeMemberMutation.mutateAsync(
                              member.user_id,
                            );
                            void membersQuery.refetch();
                          }}
                        >
                          Delete&hellip;
                        </MoreMenuItem>
                      </MoreMenuContent>
                    </MoreMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Stack>
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

interface AddGroupMemberProps {
  isLoading: boolean;
  onSubmit: (user: User) => Promise<void>;
}

const AddGroupMember: FC<AddGroupMemberProps> = ({ isLoading, onSubmit }) => {
  const [selectedUser, setSelectedUser] = useState<User | null>(null);

  return (
    <form
      onSubmit={async (e) => {
        e.preventDefault();

        if (selectedUser) {
          try {
            await onSubmit(selectedUser);
            setSelectedUser(null);
          } catch (error) {
            displayError(getErrorMessage(error, "Failed to add member."));
          }
        }
      }}
    >
      <Stack direction="row" alignItems="center" spacing={1}>
        <UserAutocomplete
          css={styles.autoComplete}
          value={selectedUser}
          onChange={(newValue) => {
            setSelectedUser(newValue);
          }}
        />

        <LoadingButton
          loadingPosition="start"
          disabled={!selectedUser}
          type="submit"
          startIcon={<PersonAdd />}
          loading={isLoading}
        >
          Add user
        </LoadingButton>
      </Stack>
    </form>
  );
};

const styles = {
  autoComplete: {
    width: 300,
  },
} satisfies Record<string, Interpolation<Theme>>;
