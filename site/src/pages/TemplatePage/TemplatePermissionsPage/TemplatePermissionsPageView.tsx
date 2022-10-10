import MenuItem from "@material-ui/core/MenuItem"
import Select from "@material-ui/core/Select"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import PersonAdd from "@material-ui/icons/PersonAdd"
import {
  Group,
  TemplateACL,
  TemplateGroup,
  TemplateRole,
  TemplateUser,
} from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { EmptyState } from "components/EmptyState/EmptyState"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { TableRowMenu } from "components/TableRowMenu/TableRowMenu"
import {
  UserOrGroupAutocomplete,
  UserOrGroupAutocompleteValue,
} from "components/UserOrGroupAutocomplete/UserOrGroupAutocomplete"
import { FC, useState } from "react"
import { Maybe } from "components/Conditionals/Maybe"

type AddTemplateUserOrGroupProps = {
  organizationId: string
  isLoading: boolean
  templateACL: TemplateACL | undefined
  onSubmit: (
    userOrGroup: TemplateUser | TemplateGroup,
    role: TemplateRole,
    reset: () => void,
  ) => void
}

const AddTemplateUserOrGroup: React.FC<AddTemplateUserOrGroupProps> = ({
  isLoading,
  onSubmit,
  organizationId,
  templateACL,
}) => {
  const styles = useStyles()
  const [selectedOption, setSelectedOption] =
    useState<UserOrGroupAutocompleteValue>(null)
  const [selectedRole, setSelectedRole] = useState<TemplateRole>("view")
  const excludeFromAutocomplete = templateACL
    ? [...templateACL.group, ...templateACL.users]
    : []

  const resetValues = () => {
    setSelectedOption(null)
    setSelectedRole("view")
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()

        if (selectedOption && selectedRole) {
          onSubmit(
            {
              ...selectedOption,
              role: selectedRole,
            },
            selectedRole,
            resetValues,
          )
        }
      }}
    >
      <Stack direction="row" alignItems="center" spacing={1}>
        <UserOrGroupAutocomplete
          exclude={excludeFromAutocomplete}
          organizationId={organizationId}
          value={selectedOption}
          onChange={(newValue) => {
            setSelectedOption(newValue)
          }}
        />

        <Select
          defaultValue="view"
          variant="outlined"
          className={styles.select}
          disabled={isLoading}
          onChange={(event) => {
            setSelectedRole(event.target.value as TemplateRole)
          }}
        >
          <MenuItem key="view" value="view">
            View
          </MenuItem>
          <MenuItem key="admin" value="admin">
            Admin
          </MenuItem>
        </Select>

        <LoadingButton
          disabled={!selectedRole || !selectedOption}
          type="submit"
          size="small"
          startIcon={<PersonAdd />}
          loading={isLoading}
        >
          Add member
        </LoadingButton>
      </Stack>
    </form>
  )
}

export interface TemplatePermissionsPageViewProps {
  templateACL: TemplateACL | undefined
  organizationId: string
  canUpdatePermissions: boolean
  // User
  onAddUser: (user: TemplateUser, role: TemplateRole, reset: () => void) => void
  isAddingUser: boolean
  onUpdateUser: (user: TemplateUser, role: TemplateRole) => void
  updatingUser: TemplateUser | undefined
  onRemoveUser: (user: TemplateUser) => void
  // Group
  onAddGroup: (
    group: TemplateGroup,
    role: TemplateRole,
    reset: () => void,
  ) => void
  isAddingGroup: boolean
  onUpdateGroup: (group: TemplateGroup, role: TemplateRole) => void
  updatingGroup: TemplateGroup | undefined
  onRemoveGroup: (group: Group) => void
}

export const TemplatePermissionsPageView: FC<
  React.PropsWithChildren<TemplatePermissionsPageViewProps>
> = ({
  templateACL,
  canUpdatePermissions,
  organizationId,
  // User
  onAddUser,
  isAddingUser,
  updatingUser,
  onUpdateUser,
  onRemoveUser,
  // Group
  onAddGroup,
  isAddingGroup,
  updatingGroup,
  onUpdateGroup,
  onRemoveGroup,
}) => {
  const styles = useStyles()
  const isEmpty = Boolean(
    templateACL &&
      templateACL.users.length === 0 &&
      templateACL.group.length === 0,
  )

  return (
    <Stack spacing={2.5}>
      <Maybe condition={canUpdatePermissions}>
        <AddTemplateUserOrGroup
          templateACL={templateACL}
          organizationId={organizationId}
          isLoading={isAddingUser || isAddingGroup}
          onSubmit={(value, role, resetAutocomplete) =>
            "members" in value
              ? onAddGroup(value, role, resetAutocomplete)
              : onAddUser(value, role, resetAutocomplete)
          }
        />
      </Maybe>
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="60%">Member</TableCell>
              <TableCell width="40%">Role</TableCell>
              <TableCell width="1%" />
            </TableRow>
          </TableHead>
          <TableBody>
            <ChooseOne>
              <Cond condition={!templateACL}>
                <TableLoader />
              </Cond>
              <Cond condition={isEmpty}>
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
                {templateACL?.group.map((group) => (
                  <TableRow key={group.id}>
                    <TableCell>
                      <AvatarData
                        title={group.name}
                        subtitle={`${group.members.length} members`}
                        highlightTitle
                      />
                    </TableCell>
                    <TableCell>
                      <ChooseOne>
                        <Cond condition={canUpdatePermissions}>
                          <Select
                            value={group.role}
                            variant="outlined"
                            className={styles.updateSelect}
                            disabled={
                              updatingGroup && updatingGroup.id === group.id
                            }
                            onChange={(event) => {
                              onUpdateGroup(
                                group,
                                event.target.value as TemplateRole,
                              )
                            }}
                          >
                            <MenuItem key="view" value="view">
                              View
                            </MenuItem>
                            <MenuItem key="admin" value="admin">
                              Admin
                            </MenuItem>
                          </Select>
                        </Cond>
                        <Cond>
                          <div className={styles.role}>{group.role}</div>
                        </Cond>
                      </ChooseOne>
                    </TableCell>

                    <TableCell>
                      <Maybe condition={canUpdatePermissions}>
                        <TableRowMenu
                          data={group}
                          menuItems={[
                            {
                              label: "Remove",
                              onClick: () => onRemoveGroup(group),
                            },
                          ]}
                        />
                      </Maybe>
                    </TableCell>
                  </TableRow>
                ))}

                {templateACL?.users.map((user) => (
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
                      <ChooseOne>
                        <Cond condition={canUpdatePermissions}>
                          <Select
                            value={user.role}
                            variant="outlined"
                            className={styles.updateSelect}
                            disabled={
                              updatingUser && updatingUser.id === user.id
                            }
                            onChange={(event) => {
                              onUpdateUser(
                                user,
                                event.target.value as TemplateRole,
                              )
                            }}
                          >
                            <MenuItem key="view" value="view">
                              View
                            </MenuItem>
                            <MenuItem key="admin" value="admin">
                              Admin
                            </MenuItem>
                          </Select>
                        </Cond>
                        <Cond>
                          <div className={styles.role}>{user.role}</div>
                        </Cond>
                      </ChooseOne>
                    </TableCell>

                    <TableCell>
                      <Maybe condition={canUpdatePermissions}>
                        <TableRowMenu
                          data={user}
                          menuItems={[
                            {
                              label: "Remove",
                              onClick: () => onRemoveUser(user),
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
  )
}

export const useStyles = makeStyles((theme) => {
  return {
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

    role: {
      textTransform: "capitalize",
    },
  }
})
