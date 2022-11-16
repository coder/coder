import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { Link as RouterLink } from "react-router-dom"
import { workspaceFilterQuery } from "../../util/filters"
import { WorkspaceItemMachineRef } from "../../xServices/workspaces/workspacesXService"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoader } from "../TableLoader/TableLoader"
import { WorkspacesRow } from "./WorkspacesRow"

interface TableBodyProps {
  isLoading?: boolean
  workspaceRefs?: WorkspaceItemMachineRef[]
  filter?: string
  isNonInitialPage: boolean
}

export const WorkspacesTableBody: FC<
  React.PropsWithChildren<TableBodyProps>
> = ({ isLoading, workspaceRefs, filter, isNonInitialPage }) => {
  const { t } = useTranslation("workspacesPage")

  return (
    <ChooseOne>
      <Cond condition={Boolean(isLoading)}>
        <TableLoader />
      </Cond>
      <Cond condition={!workspaceRefs || workspaceRefs.length === 0}>
        <TableRow>
          <TableCell colSpan={999}>
            <ChooseOne>
              <Cond condition={isNonInitialPage}>
                <EmptyState message={t("emptyPageMessage")} />
              </Cond>
              <Cond
                condition={
                  filter === workspaceFilterQuery.me ||
                  filter === workspaceFilterQuery.all
                }
              >
                <EmptyState
                  message={t("emptyCreateWorkspaceMessage")}
                  description={t("emptyCreateWorkspaceDescription")}
                  cta={
                    <Link
                      underline="none"
                      component={RouterLink}
                      to="/templates"
                    >
                      <Button startIcon={<AddCircleOutline />}>
                        {t("createFromTemplateButton")}
                      </Button>
                    </Link>
                  }
                />
              </Cond>
              <Cond>
                <EmptyState message={t("emptyResultsMessage")} />
              </Cond>
            </ChooseOne>
          </TableCell>
        </TableRow>
      </Cond>
      <Cond>
        {workspaceRefs &&
          workspaceRefs.map((workspaceRef) => (
            <WorkspacesRow workspaceRef={workspaceRef} key={workspaceRef.id} />
          ))}
      </Cond>
    </ChooseOne>
  )
}
