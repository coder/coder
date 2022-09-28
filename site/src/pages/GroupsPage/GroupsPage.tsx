import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import AvatarGroup from "@material-ui/lab/AvatarGroup"
import { useMachine } from "@xstate/react"
import { AvatarData } from "components/AvatarData/AvatarData"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { EmptyState } from "components/EmptyState/EmptyState"
import { TableLoader } from "components/TableLoader/TableLoader"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { useOrganizationId } from "hooks/useOrganizationId"
import React from "react"
import { Helmet } from "react-helmet-async"
import { Link as RouterLink, useNavigate } from "react-router-dom"
import { pageTitle } from "util/page"
import { groupsMachine } from "xServices/groups/groupsXService"

export const GroupsPage: React.FC = () => {
  const organizationId = useOrganizationId()
  const [state] = useMachine(groupsMachine, {
    context: {
      organizationId,
    },
  })
  const { groups } = state.context
  const isLoading = Boolean(groups === undefined)
  const isEmpty = Boolean(groups && groups.length === 0)
  const navigate = useNavigate()
  const styles = useStyles()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Groups")}</title>
      </Helmet>
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
                <TableLoader />
              </Cond>

              <Cond condition={isEmpty}>
                <TableRow>
                  <TableCell colSpan={999}>
                    <EmptyState
                      message="No groups yet"
                      description="Create your first group"
                      cta={
                        <Link underline="none" component={RouterLink} to="/groups/create">
                          <Button startIcon={<AddCircleOutline />}>Create group</Button>
                        </Link>
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
                          title={group.name}
                          subtitle={`${group.members.length} members`}
                          highlightTitle
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
                          <KeyboardArrowRight className={styles.arrowRight} />
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

export default GroupsPage
