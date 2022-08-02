import Box from "@material-ui/core/Box"
import { fade, makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import useTheme from "@material-ui/styles/useTheme"
import { FC } from "react"
import { useNavigate, useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { displayWorkspaceBuildDuration, getDisplayWorkspaceBuildStatus } from "../../util/workspace"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableCellLink } from "../TableCellLink/TableCellLink"
import { TableLoader } from "../TableLoader/TableLoader"

export const Language = {
  emptyMessage: "No builds found",
  inProgressLabel: "In progress",
  actionLabel: "Action",
  durationLabel: "Duration",
  startedAtLabel: "Started at",
  statusLabel: "Status",
}

export interface BuildsTableProps {
  builds?: TypesGen.WorkspaceBuild[]
  className?: string
}

export const BuildsTable: FC<React.PropsWithChildren<BuildsTableProps>> = ({ builds, className }) => {
  const { username, workspace: workspaceName } = useParams()
  const isLoading = !builds
  const theme: Theme = useTheme()
  const navigate = useNavigate()
  const styles = useStyles()

  return (
    <TableContainer className={className}>
      <Table data-testid="builds-table" aria-describedby="builds table">
        <TableHead>
          <TableRow>
            <TableCell width="20%">{Language.actionLabel}</TableCell>
            <TableCell width="20%">{Language.durationLabel}</TableCell>
            <TableCell width="40%">{Language.startedAtLabel}</TableCell>
            <TableCell width="20%">{Language.statusLabel}</TableCell>
            <TableCell></TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {isLoading && <TableLoader />}
          {builds &&
            builds.map((build) => {
              const status = getDisplayWorkspaceBuildStatus(theme, build)
              const buildPageLink = `/@${username}/${workspaceName}/builds/${build.build_number}`

              return (
                <TableRow
                  hover
                  key={build.id}
                  data-testid={`build-${build.id}`}
                  tabIndex={0}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      navigate(buildPageLink)
                    }
                  }}
                  className={styles.clickableTableRow}
                >
                  <TableCellLink to={buildPageLink}>{build.transition}</TableCellLink>
                  <TableCellLink to={buildPageLink}>
                    <span style={{ color: theme.palette.text.secondary }}>
                      {displayWorkspaceBuildDuration(build)}
                    </span>
                  </TableCellLink>
                  <TableCellLink to={buildPageLink}>
                    <span style={{ color: theme.palette.text.secondary }}>
                      {new Date(build.created_at).toLocaleString()}
                    </span>
                  </TableCellLink>
                  <TableCellLink to={buildPageLink}>
                    <span style={{ color: status.color }} className={styles.status}>
                      {status.status}
                    </span>
                  </TableCellLink>
                  <TableCellLink to={buildPageLink}>
                    <div className={styles.arrowCell}>
                      <KeyboardArrowRight className={styles.arrowRight} />
                    </div>
                  </TableCellLink>
                </TableRow>
              )
            })}

          {builds && builds.length === 0 && (
            <TableRow>
              <TableCell colSpan={999}>
                <Box p={4}>
                  <EmptyState message={Language.emptyMessage} />
                </Box>
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

const useStyles = makeStyles((theme) => ({
  clickableTableRow: {
    "&:hover td": {
      backgroundColor: fade(theme.palette.primary.dark, 0.1),
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: theme.spacing(2),
    },
  },
  arrowRight: {
    color: fade(theme.palette.primary.contrastText, 0.7),
    width: 20,
    height: 20,
  },
  arrowCell: {
    display: "flex",
  },
  status: {
    whiteSpace: "nowrap",
  },
}))
