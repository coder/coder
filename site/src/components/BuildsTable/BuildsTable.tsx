import Box from "@material-ui/core/Box"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableRow from "@material-ui/core/TableRow"
import { FC, Fragment } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoader } from "../TableLoader/TableLoader"
import { BuildDateRow } from "./BuildDateRow"
import { BuildRow } from "./BuildRow"

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

const groupBuildsByDate = (builds?: TypesGen.WorkspaceBuild[]) => {
  const buildsByDate: Record<string, TypesGen.WorkspaceBuild[]> = {}

  if (!builds) {
    return
  }

  builds.forEach((build) => {
    const dateKey = new Date(build.created_at).toDateString()

    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
    if (buildsByDate[dateKey]) {
      buildsByDate[dateKey].push(build)
    } else {
      buildsByDate[dateKey] = [build]
    }
  })

  return buildsByDate
}

export const BuildsTable: FC<React.PropsWithChildren<BuildsTableProps>> = ({
  builds,
  className,
}) => {
  const isLoading = !builds
  const buildsByDate = groupBuildsByDate(builds)

  return (
    <TableContainer className={className}>
      <Table data-testid="builds-table" aria-describedby="builds table">
        <TableBody>
          {isLoading && <TableLoader />}

          {buildsByDate &&
            Object.keys(buildsByDate).map((dateStr) => {
              const builds = buildsByDate[dateStr]

              return (
                <Fragment key={dateStr}>
                  <BuildDateRow date={new Date(dateStr)} />
                  {builds.map((build) => (
                    <BuildRow key={build.id} build={build} />
                  ))}
                </Fragment>
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
