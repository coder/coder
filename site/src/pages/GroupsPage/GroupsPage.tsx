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
import { useMachine } from "@xstate/react"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { EmptyState } from "components/EmptyState/EmptyState"
import { Margins } from "components/Margins/Margins"
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { TableCellLink } from "components/TableCellLink/TableCellLink"
import { TableLoader } from "components/TableLoader/TableLoader"
import { useOrganizationId } from "hooks/useOrganizationId"
import React from "react"
import { Helmet } from "react-helmet-async"
import { Link as RouterLink, useNavigate } from "react-router-dom"
import { pageTitle } from "util/page"
import { groupsMachine } from "xServices/groups/groupsXService"

const CreateGroupButton: React.FC = () => {
  return (
    <Link underline="none" component={RouterLink} to="/groups/new">
      <Button startIcon={<AddCircleOutline />}>Create group</Button>
    </Link>
  )
}

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
      <Margins>
        <PageHeader actions={<CreateGroupButton />}>
          <PageHeaderTitle>Groups</PageHeaderTitle>
        </PageHeader>
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
                        cta={<CreateGroupButton />}
                      />
                    </TableCell>
                  </TableRow>
                </Cond>

                <Cond condition={!isEmpty}>
                  {groups?.map((group) => {
                    const groupPageLink = `/groups/${group.id}`

                    return (
                      <TableRow
                        key={group.id}
                        hover
                        data-testid={`group-${group.id}`}
                        tabIndex={0}
                        onKeyDown={(event) => {
                          if (event.key === "Enter") {
                            navigate(groupPageLink)
                          }
                        }}
                        className={styles.clickableTableRow}
                      >
                        <TableCellLink to={groupPageLink}>{group.name}</TableCellLink>

                        <TableCell>Users</TableCell>

                        <TableCellLink to={groupPageLink}>
                          <div className={styles.arrowCell}>
                            <KeyboardArrowRight className={styles.arrowRight} />
                          </div>
                        </TableCellLink>
                      </TableRow>
                    )
                  })}
                </Cond>
              </ChooseOne>
            </TableBody>
          </Table>
        </TableContainer>
      </Margins>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  clickableTableRow: {
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
