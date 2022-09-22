import CircularProgress from "@material-ui/core/CircularProgress"
import MenuItem from "@material-ui/core/MenuItem"
import Select from "@material-ui/core/Select"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import TextField from "@material-ui/core/TextField"
import PersonAdd from "@material-ui/icons/PersonAdd"
import Autocomplete from "@material-ui/lab/Autocomplete"
import { useMachine } from "@xstate/react"
import { TemplateRole, TemplateUser, User } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { EmptyState } from "components/EmptyState/EmptyState"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { TableRowMenu } from "components/TableRowMenu/TableRowMenu"
import debounce from "just-debounce-it"
import { ChangeEvent, FC, useState } from "react"
import { searchUserMachine } from "xServices/users/searchUserXService"

const AddTemplateUser: React.FC<{
  isLoading: boolean
  onSubmit: (user: User, role: TemplateRole, reset: () => void) => void
}> = ({ isLoading, onSubmit }) => {
  const styles = useStyles()
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false)
  const [searchState, sendSearch] = useMachine(searchUserMachine)
  const { searchResults } = searchState.context
  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [selectedRole, setSelectedRole] = useState<TemplateRole>("read")

  const handleFilterChange = debounce((event: ChangeEvent<HTMLInputElement>) => {
    sendSearch("SEARCH", { query: event.target.value })
  }, 1000)

  const resetValues = () => {
    setSelectedUser(null)
    setSelectedRole("read")
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()

        if (selectedUser && selectedRole) {
          onSubmit(selectedUser, selectedRole, resetValues)
        }
      }}
    >
      <Stack direction="row" alignItems="center" spacing={1}>
        <Autocomplete
          value={selectedUser}
          disabled={isLoading}
          id="asynchronous-demo"
          style={{ width: 300 }}
          open={isAutocompleteOpen}
          onOpen={() => {
            setIsAutocompleteOpen(true)
          }}
          onClose={() => {
            setIsAutocompleteOpen(false)
          }}
          onChange={(event, newValue) => {
            setSelectedUser(newValue)
          }}
          getOptionSelected={(option: User, value: User) => option.username === value.username}
          getOptionLabel={(option) => option.email}
          renderOption={(option: User) => (
            <AvatarData
              title={option.username}
              subtitle={option.email}
              highlightTitle
              avatar={
                option.avatar_url ? (
                  <img
                    className={styles.avatar}
                    alt={`${option.username}'s Avatar`}
                    src={option.avatar_url}
                  />
                ) : null
              }
            />
          )}
          options={searchResults}
          loading={searchState.matches("searching")}
          className={styles.autocomplete}
          renderInput={(params) => (
            <TextField
              {...params}
              margin="none"
              variant="outlined"
              placeholder="User email or username"
              InputProps={{
                ...params.InputProps,
                onChange: handleFilterChange,
                endAdornment: (
                  <>
                    {searchState.matches("searching") ? <CircularProgress size={16} /> : null}
                    {params.InputProps.endAdornment}
                  </>
                ),
              }}
            />
          )}
        />

        <Select
          defaultValue="read"
          variant="outlined"
          className={styles.select}
          disabled={isLoading}
          onChange={(event) => {
            setSelectedRole(event.target.value as TemplateRole)
          }}
        >
          <MenuItem key="read" value="read">
            Read
          </MenuItem>
          <MenuItem key="write" value="write">
            Write
          </MenuItem>
          <MenuItem key="admin" value="admin">
            Admin
          </MenuItem>
        </Select>

        <LoadingButton
          disabled={!selectedRole || !selectedUser}
          type="submit"
          size="small"
          startIcon={<PersonAdd />}
          loading={isLoading}
        >
          Add collaborator
        </LoadingButton>
      </Stack>
    </form>
  )
}

export interface TemplateCollaboratorsPageViewProps {
  deleteTemplateError: Error | unknown
  templateUsers: TemplateUser[] | undefined
  onAddUser: (user: User, role: TemplateRole, reset: () => void) => void
  isAddingUser: boolean
  canUpdateUsers: boolean
  onUpdateUser: (user: User, role: TemplateRole) => void
  updatingUser: TemplateUser | undefined
  onRemoveUser: (user: User) => void
}

export const TemplateCollaboratorsPageView: FC<
  React.PropsWithChildren<TemplateCollaboratorsPageViewProps>
> = ({
  deleteTemplateError,
  templateUsers,
  onAddUser,
  isAddingUser,
  updatingUser,
  onUpdateUser,
  canUpdateUsers,
  onRemoveUser,
}) => {
  const styles = useStyles()
  const deleteError = deleteTemplateError ? (
    <ErrorSummary error={deleteTemplateError} dismissible />
  ) : null

  return (
    <Stack spacing={2.5}>
      {deleteError}
      <AddTemplateUser isLoading={isAddingUser} onSubmit={onAddUser} />
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="60%">User</TableCell>
              <TableCell width="40%">Role</TableCell>
              <TableCell width="1%" />
            </TableRow>
          </TableHead>
          <TableBody>
            <ChooseOne>
              <Cond condition={!templateUsers}>
                <TableLoader />
              </Cond>
              <Cond condition={Boolean(templateUsers && templateUsers.length === 0)}>
                <TableRow>
                  <TableCell colSpan={999}>
                    <EmptyState
                      message="No collaborators yet"
                      description="Add a collaborator using the controls above"
                    />
                  </TableCell>
                </TableRow>
              </Cond>
              <Cond condition={Boolean(templateUsers && templateUsers.length > 0)}>
                {templateUsers?.map((user) => (
                  <TableRow key={user.id}>
                    <TableCell>
                      <AvatarData
                        title={user.username}
                        subtitle={user.email}
                        highlightTitle
                        avatar={
                          user.avatar_url ? (
                            <img
                              className={styles.avatar}
                              alt={`${user.username}'s Avatar`}
                              src={user.avatar_url}
                            />
                          ) : null
                        }
                      />
                    </TableCell>
                    <TableCell>
                      {canUpdateUsers ? (
                        <Select
                          value={user.role}
                          variant="outlined"
                          className={styles.updateSelect}
                          disabled={updatingUser && updatingUser.id === user.id}
                          onChange={(event) => {
                            onUpdateUser(user, event.target.value as TemplateRole)
                          }}
                        >
                          <MenuItem key="read" value="read">
                            Read
                          </MenuItem>
                          <MenuItem key="write" value="write">
                            Write
                          </MenuItem>
                          <MenuItem key="admin" value="admin">
                            Admin
                          </MenuItem>
                        </Select>
                      ) : (
                        user.role
                      )}
                    </TableCell>

                    {canUpdateUsers && (
                      <TableCell>
                        <TableRowMenu
                          data={user}
                          menuItems={[
                            {
                              label: "Remove",
                              onClick: () => onRemoveUser(user),
                            },
                          ]}
                        />
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </Cond>
            </ChooseOne>
          </TableBody>
        </Table>
      </TableContainer>
    </Stack>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    autocomplete: {
      "& .MuiInputBase-root": {
        width: 300,
        // Match button small height
        height: 36,
      },

      "& input": {
        fontSize: 14,
        padding: `${theme.spacing(0, 0.5, 0, 0.5)} !important`,
      },
    },

    select: {
      // Match button small height
      height: 36,
      fontSize: 14,
      width: 100,
    },

    avatar: {
      width: theme.spacing(4.5),
      height: theme.spacing(4.5),
      borderRadius: "100%",
    },

    updateSelect: {
      margin: 0,
      // Set a fixed width for the select. It avoids selects having different sizes
      // depending on how many roles they have selected.
      width: theme.spacing(25),
      "& .MuiSelect-root": {
        // Adjusting padding because it does not have label
        paddingTop: theme.spacing(1.5),
        paddingBottom: theme.spacing(1.5),
      },
    },
  }
})
