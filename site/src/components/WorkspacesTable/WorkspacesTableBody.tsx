import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import AddOutlined from "@material-ui/icons/AddOutlined"
import { Workspace } from "api/typesGenerated"
import { TableEmpty } from "components/TableEmpty/TableEmpty"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { Link as RouterLink } from "react-router-dom"
import { TableLoader } from "../TableLoader/TableLoader"
import { WorkspacesRow } from "./WorkspacesRow"

interface TableBodyProps {
  workspaces?: Workspace[]
  isUsingFilter: boolean
  onUpdateWorkspace: (workspace: Workspace) => void
}

export const WorkspacesTableBody: FC<
  React.PropsWithChildren<TableBodyProps>
> = ({ workspaces, isUsingFilter, onUpdateWorkspace }) => {
  const { t } = useTranslation("workspacesPage")
  const styles = useStyles()

  if (!workspaces) {
    return <TableLoader />
  }

  if (workspaces.length === 0) {
    return isUsingFilter ? (
      <TableEmpty message={t("emptyPageMessage")} />
    ) : (
      <TableEmpty
        message={t("emptyCreateWorkspaceMessage")}
        description={t("emptyCreateWorkspaceDescription")}
        cta={
          <Link underline="none" component={RouterLink} to="/templates">
            <Button startIcon={<AddOutlined />}>
              {t("createFromTemplateButton")}
            </Button>
          </Link>
        }
        image={
          <div className={styles.emptyImage}>
            <img src="/featured/workspaces.webp" alt="" />
          </div>
        }
      />
    )
  }

  return (
    <>
      {workspaces.map((workspace) => (
        <WorkspacesRow
          workspace={workspace}
          key={workspace.id}
          onUpdateWorkspace={onUpdateWorkspace}
        />
      ))}
    </>
  )
}

const useStyles = makeStyles((theme) => ({
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
