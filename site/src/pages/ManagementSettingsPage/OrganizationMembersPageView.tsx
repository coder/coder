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
import { getErrorMessage } from "api/errors";
import type {
  User,
  OrganizationMemberWithUserData,
  SlimRole,
} from "api/typesGenerated";
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
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { TableColumnHelpTooltip } from "./UserTable/TableColumnHelpTooltip";
import { UserRoleCell } from "./UserTable/UserRoleCell";

interface OrganizationMembersPageViewProps {
  allAvailableRoles: readonly SlimRole[] | undefined;
  canEditMembers: boolean;
  error: unknown;
  isAddingMember: boolean;
  isUpdatingMemberRoles: boolean;
  me: User;
  members: OrganizationMemberWithUserData[] | undefined;
  addMember: (user: User) => Promise<void>;
  removeMember: (member: OrganizationMemberWithUserData) => Promise<void>;
  updateMemberRoles: (
    member: OrganizationMemberWithUserData,
    newRoles: string[],
  ) => Promise<void>;
}

export const OrganizationMembersPageView: FC<
  OrganizationMembersPageViewProps
> = (props) => {
  return (
    <div>
      <SettingsHeader title="Organization members" />

      <Stack>
        {Boolean(props.error) && <ErrorAlert error={props.error} />}

        {props.canEditMembers && (
          <AddOrganizationMember
            isLoading={props.isAddingMember}
            onSubmit={props.addMember}
          />
        )}

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
              {props.members?.map((member) => (
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
                    inheritedRoles={member.global_roles}
                    roles={member.roles}
                    allAvailableRoles={props.allAvailableRoles}
                    oidcRoleSyncEnabled={false}
                    isLoading={props.isUpdatingMemberRoles}
                    canEditUsers={props.canEditMembers}
                    onEditRoles={async (roles) => {
                      try {
                        await props.updateMemberRoles(member, roles);
                        displaySuccess("Roles updated successfully.");
                      } catch (error) {
                        displayError(
                          getErrorMessage(error, "Failed to update roles."),
                        );
                      }
                    }}
                  />
                  <TableCell>
                    {member.user_id !== props.me.id && props.canEditMembers && (
                      <MoreMenu>
                        <MoreMenuTrigger>
                          <ThreeDotsButton />
                        </MoreMenuTrigger>
                        <MoreMenuContent>
                          <MoreMenuItem
                            danger
                            onClick={async () => {
                              try {
                                await props.removeMember(member);
                                displaySuccess("Member removed successfully.");
                              } catch (error) {
                                displayError(
                                  getErrorMessage(
                                    error,
                                    "Failed to remove member.",
                                  ),
                                );
                              }
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
      onSubmit={async (event) => {
        event.preventDefault();

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
