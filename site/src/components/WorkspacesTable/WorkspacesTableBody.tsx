import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import AddOutlined from "@material-ui/icons/AddOutlined"
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
  const styles = useStyles()

  return (
    <ChooseOne>
      <Cond condition={Boolean(isLoading)}>
        <TableLoader />
      </Cond>
      <Cond condition={!workspaceRefs || workspaceRefs.length === 0}>
        <TableRow>
          <TableCell colSpan={999} className={styles.emptyTableCell}>
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
                  className={styles.empty}
                  message={t("emptyCreateWorkspaceMessage")}
                  description={t("emptyCreateWorkspaceDescription")}
                  cta={
                    <Link
                      underline="none"
                      component={RouterLink}
                      to="/templates"
                    >
                      <Button startIcon={<AddOutlined />}>
                        {t("createFromTemplateButton")}
                      </Button>
                    </Link>
                  }
                  image={
                    <div className={styles.emptyImage}>
                      <img src="/empty/workspaces.webp" alt="" />
                    </div>
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

const useStyles = makeStyles((theme) => ({
  emptyTableCell: {
    padding: "0 !important",
  },

  empty: {
    paddingBottom: 0,
  },

  emptyImage: {
    maxWidth: "50%",
    height: theme.spacing(34),
    overflow: "hidden",
    marginTop: theme.spacing(6),
    opacity: 0.85,

    "& img": {
      maxWidth: "100%",
    },
  },
}))
