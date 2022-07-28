import Link from "@material-ui/core/Link"
import { PaginationWidget } from "components/PaginationWidget/PaginationWidget"
import { FC, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
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
  createANewWorkspace: `Create a new workspace from a `,
  template: "Template",
}

export interface WorkspacesPageViewProps {
  isLoading?: boolean
  workspaceRefs?: WorkspaceItemMachineRef[]
  filter?: string
  onFilter: (query: string) => void
}

export const WorkspacesPageView: FC<WorkspacesPageViewProps> = ({
  isLoading,
  workspaceRefs,
  filter,
  onFilter,
}) => {
  const presetFilters = [
    { query: workspaceFilterQuery.me, name: Language.yourWorkspacesButton },
    { query: workspaceFilterQuery.all, name: Language.allWorkspacesButton },
  ]

  const [activePage, setActivePage] = useState(1)

  return (
    <div style={{ display: "flex", justifyContent: "center", padding: "40px" }}>
      <PaginationWidget
        prevLabel="Previous"
        nextLabel="Next"
        onPrevClick={() => setActivePage(activePage - 1)}
        onNextClick={() => setActivePage(activePage + 1)}
        numRecordsPerPage={15}
        numRecords={400}
        activePage={activePage}
      />
    </div>
  )

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

      <SearchBarWithFilter filter={filter} onFilter={onFilter} presetFilters={presetFilters} />

      <WorkspacesTable isLoading={isLoading} workspaceRefs={workspaceRefs} filter={filter} />
    </Margins>
  )
}
