import Button from "@mui/material/Button";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import PersonAdd from "@mui/icons-material/PersonAdd";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import type { Group, User } from "api/typesGenerated";
import { AvatarData } from "components/AvatarData/AvatarData";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { Link as RouterLink, useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import {
  PaginationStatus,
  TableToolbar,
} from "components/TableToolbar/TableToolbar";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { isEveryoneGroup } from "utils/groups";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  addMember,
  deleteGroup,
  group,
  groupPermissions,
  removeMember,
} from "api/queries/groups";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { getErrorMessage } from "api/errors";
import Box from "@mui/material/Box";
import { LastSeen } from "components/LastSeen/LastSeen";
import { type Interpolation, type Theme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";

export const GroupPage: FC = () => {
  const { groupId } = useParams() as { groupId: string };
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const groupQuery = useQuery(group(groupId));
  const groupData = groupQuery.data;
  const { data: permissions } = useQuery(groupPermissions(groupId));
  const addMemberMutation = useMutation(addMember(queryClient));
  const deleteGroupMutation = useMutation(deleteGroup(queryClient));
  const [isDeletingGroup, setIsDeletingGroup] = useState(false);
  const isLoading = !groupData || !permissions;
  const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;

  const helmet = (
    <Helmet>
      <title>
        {pageTitle(
          (groupData?.display_name || groupData?.name) ?? "Loading...",
        )}
      </title>
    </Helmet>
  );

  if (isLoading) {
    return (
      <>
        {helmet}
        <Loader />
      </>
    );
  }

  return (
    <>
      {helmet}

      <Margins>
        <PageHeader
          actions={
            canUpdateGroup && (
              <>
                <Button
                  startIcon={<SettingsOutlined />}
                  to="settings"
                  component={RouterLink}
                >
                  Settings
                </Button>
                <Button
                  disabled={groupData?.id === groupData?.organization_id}
                  onClick={() => {
                    setIsDeletingGroup(true);
                  }}
                  startIcon={<DeleteOutline />}
                  css={styles.removeButton}
                >
                  Delete&hellip;
                </Button>
              </>
            )
          }
        >
          <PageHeaderTitle>
            {groupData?.display_name || groupData?.name}
          </PageHeaderTitle>
          <PageHeaderSubtitle>
            {/* Show the name if it differs from the display name. */}
            {groupData?.display_name &&
            groupData?.display_name !== groupData?.name
              ? groupData?.name
              : ""}{" "}
          </PageHeaderSubtitle>
        </PageHeader>

        <Stack spacing={1}>
          {canUpdateGroup && groupData && !isEveryoneGroup(groupData) && (
            <AddGroupMember
              isLoading={addMemberMutation.isLoading}
              onSubmit={async (user, reset) => {
                try {
                  await addMemberMutation.mutateAsync({
                    groupId,
                    userId: user.id,
                  });
                  reset();
                } catch (error) {
                  displayError(getErrorMessage(error, "Failed to add member."));
                }
              }}
            />
          )}
          <TableToolbar>
            <PaginationStatus
              isLoading={Boolean(isLoading)}
              showing={groupData?.members.length ?? 0}
              total={groupData?.members.length ?? 0}
              label="members"
            />
          </TableToolbar>

          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell width="59%">User</TableCell>
                  <TableCell width="40">Status</TableCell>
                  <TableCell width="1%"></TableCell>
                </TableRow>
              </TableHead>

              <TableBody>
                {groupData?.members.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={999}>
                      <EmptyState
                        message="No members yet"
                        description="Add a member using the controls above"
                      />
                    </TableCell>
                  </TableRow>
                ) : (
                  groupData?.members.map((member) => (
                    <GroupMemberRow
                      member={member}
                      group={groupData}
                      key={member.id}
                      canUpdate={canUpdateGroup}
                    />
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </Stack>
      </Margins>

      {groupQuery.data && (
        <DeleteDialog
          isOpen={isDeletingGroup}
          confirmLoading={deleteGroupMutation.isLoading}
          name={groupQuery.data.name}
          entity="group"
          onConfirm={async () => {
            try {
              await deleteGroupMutation.mutateAsync(groupId);
              navigate("/groups");
            } catch (error) {
              displayError(getErrorMessage(error, "Failed to delete group."));
            }
          }}
          onCancel={() => {
            setIsDeletingGroup(false);
          }}
        />
      )}
    </>
  );
};

const AddGroupMember: React.FC<{
  isLoading: boolean;
  onSubmit: (user: User, reset: () => void) => void;
}> = ({ isLoading, onSubmit }) => {
  const [selectedUser, setSelectedUser] = useState<User | null>(null);

  const resetValues = () => {
    setSelectedUser(null);
  };

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();

        if (selectedUser) {
          onSubmit(selectedUser, resetValues);
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

const GroupMemberRow = (props: {
  member: User;
  group: Group;
  canUpdate: boolean;
}) => {
  const { member, group, canUpdate } = props;
  const queryClient = useQueryClient();
  const removeMemberMutation = useMutation(removeMember(queryClient));

  return (
    <TableRow key={member.id}>
      <TableCell width="59%">
        <AvatarData
          avatar={
            <UserAvatar
              username={member.username}
              avatarURL={member.avatar_url}
            />
          }
          title={member.username}
          subtitle={member.email}
        />
      </TableCell>
      <TableCell
        width="40%"
        css={[styles.status, member.status === "suspended" && styles.suspended]}
      >
        <Box>{member.status}</Box>
        <LastSeen value={member.last_seen_at} sx={{ fontSize: 12 }} />
      </TableCell>
      <TableCell width="1%">
        {canUpdate && (
          <MoreMenu>
            <MoreMenuTrigger>
              <ThreeDotsButton />
            </MoreMenuTrigger>
            <MoreMenuContent>
              <MoreMenuItem
                danger
                onClick={async () => {
                  try {
                    await removeMemberMutation.mutateAsync({
                      groupId: group.id,
                      userId: member.id,
                    });
                    displaySuccess("Member removed successfully.");
                  } catch (error) {
                    displayError(
                      getErrorMessage(error, "Failed to remove member."),
                    );
                  }
                }}
                disabled={group.id === group.organization_id}
              >
                Remove
              </MoreMenuItem>
            </MoreMenuContent>
          </MoreMenu>
        )}
      </TableCell>
    </TableRow>
  );
};

const styles = {
  autoComplete: {
    width: 300,
  },
  removeButton: (theme) => ({
    color: theme.palette.error.main,
    "&:hover": {
      backgroundColor: "transparent",
    },
  }),
  status: {
    textTransform: "capitalize",
  },
  suspended: (theme) => ({
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export default GroupPage;
