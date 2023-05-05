import Button from "@mui/material/Button"
import Link from "@mui/material/Link"
import { makeStyles } from "@mui/material/styles"
import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableHead from "@mui/material/TableHead"
import TableRow from "@mui/material/TableRow"
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined"
import AddCircleOutline from "@mui/icons-material/AddCircleOutline"
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight"
import AvatarGroup from '@mui/material/AvatarGroup'
import { AvatarData } from "components/AvatarData/AvatarData"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { EmptyState } from "components/EmptyState/EmptyState"
import { Stack } from "components/Stack/Stack"
import { TableLoaderSkeleton } from "components/TableLoader/TableLoader"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { FC } from "react"
import { Link as RouterLink, useNavigate } from "react-router-dom"
import { Paywall } from "components/Paywall/Paywall"
import { Group } from "api/typesGenerated"
import { GroupAvatar } from "components/GroupAvatar/GroupAvatar"

export type GroupsPageViewProps = {
  groups: Group[] | undefined
  canCreateGroup: boolean
  isTemplateRBACEnabled: boolean
}

export const GroupsPageView: FC<GroupsPageViewProps> = ({
  groups,
  canCreateGroup,
  isTemplateRBACEnabled,
}) => {
  const isLoading = Boolean(groups === undefined)
  const isEmpty = Boolean(groups && groups.length === 0)
  const navigate = useNavigate()
  const styles = useStyles()

  return (
    <>
      <ChooseOne>
        <Cond condition={!isTemplateRBACEnabled}>
          <Paywall
            message="User groups"
            description="Organize the users into groups and manage their permissions. To use this feature, you have to upgrade your account."
            cta={
              <Stack direction="row" alignItems="center">
                <Link
                  underline="none"
                  href="https://coder.com/docs/coder-oss/latest/enterprise"
                  target="_blank"
                  rel="noreferrer"
                >
                  <Button size="small" startIcon={<ArrowRightAltOutlined />}>
                    See how to upgrade
                  </Button>
                </Link>
                <Link
                  underline="none"
                  href="https://coder.com/docs/coder-oss/latest/admin/groups"
                  target="_blank"
                  rel="noreferrer"
                >
                  Read the docs
                </Link>
              </Stack>
            }
          />
        </Cond>
        <Cond>
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
                <ChooseOne>
                  <Cond condition={isLoading}>
                    <TableLoaderSkeleton columns={2} useAvatarData />
                  </Cond>

                  <Cond condition={isEmpty}>
                    <TableRow>
                      <TableCell colSpan={999}>
                        <EmptyState
                          message="No groups yet"
                          description={
                            canCreateGroup
                              ? "Create your first group"
                              : "You don't have permission to create a group"
                          }
                          cta={
                            canCreateGroup && (
                              <Link
                                underline="none"
                                component={RouterLink}
                                to="/groups/create"
                              >
                                <Button startIcon={<AddCircleOutline />}>
                                  Create group
                                </Button>
                              </Link>
                            )
                          }
                        />
                      </TableCell>
                    </TableRow>
                  </Cond>

                  <Cond>
                    {groups?.map((group) => {
                      const groupPageLink = `/groups/${group.id}`

                      return (
                        <TableRow
                          hover
                          key={group.id}
                          data-testid={`group-${group.id}`}
                          tabIndex={0}
                          onClick={() => {
                            navigate(groupPageLink)
                          }}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") {
                              navigate(groupPageLink)
                            }
                          }}
                          className={styles.clickableTableRow}
                        >
                          <TableCell>
                            <AvatarData
                              avatar={
                                <GroupAvatar
                                  name={group.name}
                                  avatarURL={group.avatar_url}
                                />
                              }
                              title={group.name}
                              subtitle={`${group.members.length} members`}
                            />
                          </TableCell>

                          <TableCell>
                            {group.members.length === 0 && "-"}
                            <AvatarGroup>
                              {group.members.map((member) => (
                                <UserAvatar
                                  key={member.username}
                                  username={member.username}
                                  avatarURL={member.avatar_url}
                                />
                              ))}
                            </AvatarGroup>
                          </TableCell>

                          <TableCell>
                            <div className={styles.arrowCell}>
                              <KeyboardArrowRight
                                className={styles.arrowRight}
                              />
                            </div>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </Cond>
                </ChooseOne>
              </TableBody>
            </Table>
          </TableContainer>
        </Cond>
      </ChooseOne>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  clickableTableRow: {
    cursor: "pointer",

    "&:hover td": {
      backgroundColor: theme.palette.action.hover,
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: theme.spacing(2),
    },
  },
  arrowRight: {
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
  },
  arrowCell: {
    display: "flex",
  },
}))

export default GroupsPageView
