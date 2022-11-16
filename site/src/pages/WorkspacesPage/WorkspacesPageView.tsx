import Link from "@material-ui/core/Link"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Maybe } from "components/Conditionals/Maybe"
import { PaginationWidget } from "components/PaginationWidget/PaginationWidget"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { PaginationMachineRef } from "xServices/pagination/paginationXService"
import { Margins } from "../../components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "../../components/PageHeader/PageHeader"
import { SearchBarWithFilter } from "../../components/SearchBarWithFilter/SearchBarWithFilter"
import { Stack } from "../../components/Stack/Stack"
import { WorkspaceHelpTooltip } from "../../components/Tooltips"
import { WorkspacesTable } from "../../components/WorkspacesTable/WorkspacesTable"
import { workspaceFilterQuery } from "../../util/filters"
import { WorkspaceItemMachineRef } from "../../xServices/workspaces/workspacesXService"

export const Language = {
  pageTitle: "Workspaces",
  yourWorkspacesButton: "Your workspaces",
  allWorkspacesButton: "All workspaces",
  runningWorkspacesButton: "Running workspaces",
  createANewWorkspace: `Create a new workspace from a `,
  template: "Template",
}

export interface WorkspacesPageViewProps {
  isLoading?: boolean
  workspaceRefs?: WorkspaceItemMachineRef[]
  count?: number
  getWorkspacesError: Error | unknown
  filter?: string
  onFilter: (query: string) => void
  paginationRef: PaginationMachineRef
  isNonInitialPage: boolean
}

export const WorkspacesPageView: FC<
  React.PropsWithChildren<WorkspacesPageViewProps>
> = ({
  isLoading,
  workspaceRefs,
  count,
  getWorkspacesError,
  filter,
  onFilter,
  paginationRef,
  isNonInitialPage,
}) => {
  const presetFilters = [
    { query: workspaceFilterQuery.me, name: Language.yourWorkspacesButton },
    { query: workspaceFilterQuery.all, name: Language.allWorkspacesButton },
    {
      query: workspaceFilterQuery.running,
      name: Language.runningWorkspacesButton,
    },
  ]

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>{Language.pageTitle}</span>
            <WorkspaceHelpTooltip />
          </Stack>
        </PageHeaderTitle>

        <PageHeaderSubtitle>
          {Language.createANewWorkspace}
          <Link component={RouterLink} to="/templates">
            {Language.template}
          </Link>
          .
        </PageHeaderSubtitle>
      </PageHeader>

      <Stack>
        <Maybe condition={getWorkspacesError !== undefined}>
          <AlertBanner
            error={getWorkspacesError}
            severity={
              workspaceRefs !== undefined && workspaceRefs.length > 0
                ? "warning"
                : "error"
            }
          />
        </Maybe>

        <SearchBarWithFilter
          filter={filter}
          onFilter={onFilter}
          presetFilters={presetFilters}
        />
      </Stack>

      <WorkspacesTable
        isLoading={isLoading}
        workspaceRefs={workspaceRefs}
        filter={filter}
        isNonInitialPage={isNonInitialPage}
      />

      <PaginationWidget numRecords={count} paginationRef={paginationRef} />
    </Margins>
  )
}
