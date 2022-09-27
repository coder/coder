import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import PersonAdd from "@material-ui/icons/PersonAdd"
import { useMachine } from "@xstate/react"
import { User } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Loader } from "components/Loader/Loader"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { Margins } from "components/Margins/Margins"
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete"
import { useState } from "react"
import { Helmet } from "react-helmet-async"
import { useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { groupMachine } from "xServices/groups/groupXService"

const AddGroupMember: React.FC<{
  isLoading: boolean
  onSubmit: (user: User, reset: () => void) => void
}> = ({ isLoading, onSubmit }) => {
  const [selectedUser, setSelectedUser] = useState<User | null>(null)

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
          value={selectedUser}
          onChange={(newValue) => {
            setSelectedUser(newValue)
          }}
        />

        <LoadingButton
          disabled={!selectedUser}
          type="submit"
          size="small"
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

  const [state, send] = useMachine(groupMachine, {
    context: {
      groupId,
    },
  })
  const { group } = state.context
  const isLoading = group === undefined

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${group?.name} · Group`)}</title>
      </Helmet>
      <ChooseOne>
        <Cond condition={isLoading}>
          <Loader />
        </Cond>

        <Cond condition>
          <Margins>
            <PageHeader>
              <PageHeaderTitle>{group?.name}</PageHeaderTitle>
            </PageHeader>

            <Stack spacing={2.5}>
              <AddGroupMember
                isLoading={state.matches("addingMember")}
                onSubmit={(user, reset) => {
                  send({ type: "ADD_MEMBER", userId: user.id, callback: reset })
                }}
              />
              <TableContainer>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell width="99%">User</TableCell>
                      <TableCell width="1%"></TableCell>
                    </TableRow>
                  </TableHead>

                  <TableBody>
                    {group?.members.map((member) => (
                      <TableRow key={member.id}>
                        <TableCell width="99%">
                          <AvatarData
                            title={member.username}
                            subtitle={member.email}
                            highlightTitle
                          />
                        </TableCell>
                        <TableCell width="1%"></TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            </Stack>
          </Margins>
        </Cond>
      </ChooseOne>
    </>
  )
}

export default GroupPage
