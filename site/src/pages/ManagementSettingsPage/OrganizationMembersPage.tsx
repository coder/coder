import type { Interpolation, Theme } from "@emotion/react";
import PersonAdd from "@mui/icons-material/PersonAdd";
import LoadingButton from "@mui/lab/LoadingButton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router-dom";
import { getErrorMessage } from "api/errors";
import {
  addOrganizationMember,
  organizationMembers,
  removeOrganizationMember,
  updateOrganizationMemberRoles,
} from "api/queries/organizations";
import { organizationRoles } from "api/queries/roles";
import type { User } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { AvatarData } from "components/AvatarData/AvatarData";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import {
  MoreMenu,
  MoreMenuTrigger,
  MoreMenuContent,
  MoreMenuItem,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { TableColumnHelpTooltip } from "./UserTable/TableColumnHelpTooltip";
import { UserRoleCell } from "./UserTable/UserRoleCell";

const OrganizationMembersPage: FC = () => {
  const queryClient = useQueryClient();
  const { organization } = useParams() as { organization: string };
  const { user: me } = useAuthenticated();

  const membersQuery = useQuery(organizationMembers(organization));
  const organizationRolesQuery = useQuery(organizationRoles(organization));

  const addMemberMutation = useMutation(
    addOrganizationMember(queryClient, organization),
  );
  const removeMemberMutation = useMutation(
    removeOrganizationMember(queryClient, organization),
  );
  const updateMemberRolesMutation = useMutation(
    updateOrganizationMemberRoles(queryClient, organization),
  );

  const error =
    membersQuery.error ?? addMemberMutation.error ?? removeMemberMutation.error;
  const members = membersQuery.data;

  return (
    <div>
      <PageHeader>
        <PageHeaderTitle>Organization members</PageHeaderTitle>
      </PageHeader>

      <Stack>
        {Boolean(error) && <ErrorAlert error={error} />}

        <AddOrganizationMember
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
                <TableCell width="49%">
                  <Stack direction="row" spacing={1} alignItems="center">
                    <span>Roles</span>
                    <TableColumnHelpTooltip variant="roles" />
                  </Stack>
                </TableCell>
                <TableCell width="1%"></TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {members?.map((member) => (
                <TableRow key={member.user_id}>
                  <TableCell>
                    <AvatarData
                      avatar={
                        <UserAvatar
                          username={member.username}
                          avatarURL={member.avatar_url}
                        />
                      }
                      title={member.name || member.username}
                      subtitle={member.email}
                    />
                  </TableCell>
                  <UserRoleCell
                    user={{
                      id: member.user_id,
                      login_type: "",
                    }}
                    inheritedRoles={member.global_roles}
                    roles={member.roles}
                    allAvailableRoles={organizationRolesQuery.data}
                    oidcRoleSyncEnabled={false}
                    isLoading={organizationRolesQuery.isLoading}
                    canEditUsers
                    onUserRolesUpdate={async (userId, newRoleNames) => {
                      await updateMemberRolesMutation.mutateAsync({
                        userId,
                        roles: newRoleNames,
                      });
                      displaySuccess("Roles updated successfully.");
                    }}
                  />
                  <TableCell>
                    {member.user_id !== me.id && (
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
                            Remove
                          </MoreMenuItem>
                        </MoreMenuContent>
                      </MoreMenu>
                    )}
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

export default OrganizationMembersPage;

interface AddOrganizationMemberProps {
  isLoading: boolean;
  onSubmit: (user: User) => Promise<void>;
}

const AddOrganizationMember: FC<AddOrganizationMemberProps> = ({
  isLoading,
  onSubmit,
}) => {
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
  role: (theme) => ({
    backgroundColor: theme.roles.info.background,
    borderColor: theme.roles.info.outline,
  }),
  globalRole: (theme) => ({
    backgroundColor: theme.roles.inactive.background,
    borderColor: theme.roles.inactive.outline,
  }),
  autoComplete: {
    width: 300,
  },
} satisfies Record<string, Interpolation<Theme>>;
