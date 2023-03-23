import MenuItem from "@material-ui/core/MenuItem"
import Select, { SelectProps } from "@material-ui/core/Select"
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
import { GroupAvatar } from "components/GroupAvatar/GroupAvatar"
import { getGroupSubtitle } from "util/groups"

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
  const [selectedRole, setSelectedRole] = useState<TemplateRole>("use")
  const excludeFromAutocomplete = templateACL
    ? [...templateACL.group, ...templateACL.users]
    : []

  const resetValues = () => {
    setSelectedOption(null)
    setSelectedRole("use")
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
          defaultValue="use"
          variant="outlined"
          className={styles.select}
          disabled={isLoading}
          onChange={(event) => {
            setSelectedRole(event.target.value as TemplateRole)
          }}
        >
          <MenuItem key="use" value="use">
            Use
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

const RoleSelect: FC<SelectProps> = (props) => {
  const styles = useStyles()

  return (
    <Select
      renderValue={(value) => <div className={styles.role}>{`${value}`}</div>}
      variant="outlined"
      className={styles.updateSelect}
      {...props}
    >
      <MenuItem key="use" value="use" className={styles.menuItem}>
        <div>
          <div>Use</div>
          <div className={styles.menuItemSecondary}>
            Can read and use this template to create workspaces.
          </div>
        </div>
      </MenuItem>
      <MenuItem key="admin" value="admin" className={styles.menuItem}>
        <div>
          <div>Admin</div>
          <div className={styles.menuItemSecondary}>
            Can modify all aspects of this template including permissions,
            metadata, and template versions.
          </div>
        </div>
      </MenuItem>
    </Select>
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
                        avatar={
                          <GroupAvatar
                            name={group.name}
                            avatarURL={group.avatar_url}
                          />
                        }
                        title={group.name}
                        subtitle={getGroupSubtitle(group)}
                      />
                    </TableCell>
                    <TableCell>
                      <ChooseOne>
                        <Cond condition={canUpdatePermissions}>
                          <RoleSelect
                            value={group.role}
                            disabled={
                              updatingGroup && updatingGroup.id === group.id
                            }
                            onChange={(event) => {
                              onUpdateGroup(
                                group,
                                event.target.value as TemplateRole,
                              )
                            }}
                          />
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
                              disabled: false,
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
                        src={user.avatar_url}
                      />
                    </TableCell>
                    <TableCell>
                      <ChooseOne>
                        <Cond condition={canUpdatePermissions}>
                          <RoleSelect
                            value={user.role}
                            disabled={
                              updatingUser && updatingUser.id === user.id
                            }
                            onChange={(event) => {
                              onUpdateUser(
                                user,
                                event.target.value as TemplateRole,
                              )
                            }}
                          />
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

    updateSelect: {
      margin: 0,
      // Set a fixed width for the select. It avoids selects having different sizes
      // depending on how many roles they have selected.
      width: theme.spacing(25),

      "& .MuiSelect-root": {
        // Adjusting padding because it does not have label
        paddingTop: theme.spacing(1.5),
        paddingBottom: theme.spacing(1.5),

        ".secondary": {
          display: "none",
        },
      },
    },

    role: {
      textTransform: "capitalize",
    },

    menuItem: {
      lineHeight: "140%",
      paddingTop: theme.spacing(1.5),
      paddingBottom: theme.spacing(1.5),
      whiteSpace: "normal",
      inlineSize: "250px",
    },

    menuItemSecondary: {
      fontSize: 14,
      color: theme.palette.text.secondary,
    },
  }
})
