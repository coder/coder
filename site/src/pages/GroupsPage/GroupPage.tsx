import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import DeleteOutline from "@material-ui/icons/DeleteOutline"
import PersonAdd from "@material-ui/icons/PersonAdd"
import SettingsOutlined from "@material-ui/icons/SettingsOutlined"
import { useMachine } from "@xstate/react"
import { User } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"
import { EmptyState } from "components/EmptyState/EmptyState"
import { Loader } from "components/Loader/Loader"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { TableRowMenu } from "components/TableRowMenu/TableRowMenu"
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete"
import { useState } from "react"
import { Helmet } from "react-helmet-async"
import { Link as RouterLink, useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { groupMachine } from "xServices/groups/groupXService"
import { Maybe } from "components/Conditionals/Maybe"
import { makeStyles } from "@material-ui/core/styles"

const AddGroupMember: React.FC<{
  isLoading: boolean
  onSubmit: (user: User, reset: () => void) => void
}> = ({ isLoading, onSubmit }) => {
  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const styles = useStyles()

  const resetValues = () => {
    setSelectedUser(null)
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()

        if (selectedUser) {
          onSubmit(selectedUser, resetValues)
        }
      }}
    >
      <Stack direction="row" alignItems="center" spacing={1}>
        <UserAutocomplete
          className={styles.autoComplete}
          value={selectedUser}
          onChange={(newValue) => {
            setSelectedUser(newValue)
          }}
        />

        <LoadingButton
          disabled={!selectedUser}
          type="submit"
          startIcon={<PersonAdd />}
          loading={isLoading}
        >
          Add user
        </LoadingButton>
      </Stack>
    </form>
  )
}

export const GroupPage: React.FC = () => {
  const { groupId } = useParams()
  if (!groupId) {
    throw new Error("groupId is not defined.")
  }

  const navigate = useNavigate()
  const [state, send] = useMachine(groupMachine, {
    context: {
      groupId,
    },
    actions: {
      redirectToGroups: () => {
        navigate("/groups")
      },
    },
  })
  const { group, permissions } = state.context
  const isLoading = group === undefined || permissions === undefined
  const canUpdateGroup = permissions ? permissions.canUpdateGroup : false

  return (
    <>
      <Helmet>
        <title>{pageTitle(group?.name ?? "Loading...")}</title>
      </Helmet>
      <ChooseOne>
        <Cond condition={isLoading}>
          <Loader />
        </Cond>

        <Cond>
          <Margins>
            <PageHeader
              actions={
                <Maybe condition={canUpdateGroup}>
                  <Link to="settings" underline="none" component={RouterLink}>
                    <Button variant="outlined" startIcon={<SettingsOutlined />}>
                      Settings
                    </Button>
                  </Link>
                  <Button
                    variant="outlined"
                    onClick={() => {
                      send("DELETE")
                    }}
                    startIcon={<DeleteOutline />}
                  >
                    Delete
                  </Button>
                </Maybe>
              }
            >
              <PageHeaderTitle>{group?.name}</PageHeaderTitle>
              <PageHeaderSubtitle>
                {group?.members.length} members
              </PageHeaderSubtitle>
            </PageHeader>

            <Stack spacing={2.5}>
              <Maybe condition={canUpdateGroup}>
                <AddGroupMember
                  isLoading={state.matches("addingMember")}
                  onSubmit={(user, reset) => {
                    send({
                      type: "ADD_MEMBER",
                      userId: user.id,
                      callback: reset,
                    })
                  }}
                />
              </Maybe>
              <TableContainer>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell width="99%">User</TableCell>
                      <TableCell width="1%"></TableCell>
                    </TableRow>
                  </TableHead>

                  <TableBody>
                    <ChooseOne>
                      <Cond condition={Boolean(group?.members.length === 0)}>
                        <TableRow>
                          <TableCell colSpan={999}>
                            <EmptyState
                              message="No members yet"
                              description="Add a member using the controls above"
                            />
                          </TableCell>
                        </TableRow>
                      </Cond>

                      <Cond>
                        {group?.members.map((member) => (
                          <TableRow key={member.id}>
                            <TableCell width="99%">
                              <AvatarData
                                title={member.username}
                                subtitle={member.email}
                              />
                            </TableCell>
                            <TableCell width="1%">
                              <Maybe condition={canUpdateGroup}>
                                <TableRowMenu
                                  data={member}
                                  menuItems={[
                                    {
                                      label: "Remove",
                                      onClick: () => {
                                        send({
                                          type: "REMOVE_MEMBER",
                                          userId: member.id,
                                        })
                                      },
                                      disabled: false,
                                    },
                                  ]}
                                />
                              </Maybe>
                            </TableCell>
                          </TableRow>
                        ))}
                      </Cond>
                    </ChooseOne>
                  </TableBody>
                </Table>
              </TableContainer>
            </Stack>
          </Margins>
        </Cond>
      </ChooseOne>

      {group && (
        <DeleteDialog
          isOpen={state.matches("confirmingDelete")}
          confirmLoading={state.matches("deleting")}
          name={group.name}
          entity="group"
          onConfirm={() => {
            send("CONFIRM_DELETE")
          }}
          onCancel={() => {
            send("CANCEL_DELETE")
          }}
        />
      )}
    </>
  )
}

const useStyles = makeStyles(() => ({
  autoComplete: {
    width: 300,
  },
}))

export default GroupPage
