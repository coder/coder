import { makeStyles } from "@mui/styles"
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { ComponentProps, FC } from "react"
import { useTranslation } from "react-i18next"
import { WorkspaceSettingsForm } from "./WorkspaceSettingsForm"
import { Workspace } from "api/typesGenerated"

export type WorkspaceSettingsPageViewProps = {
  error: unknown
  isSubmitting: boolean
  workspace: Workspace
  onCancel: () => void
  onSubmit: ComponentProps<typeof WorkspaceSettingsForm>["onSubmit"]
}

export const WorkspaceSettingsPageView: FC<WorkspaceSettingsPageViewProps> = ({
  onCancel,
  onSubmit,
  isSubmitting,
  error,
  workspace,
}) => {
  const { t } = useTranslation("workspaceSettingsPage")
  const styles = useStyles()

  return (
    <>
      <PageHeader className={styles.pageHeader}>
        <PageHeaderTitle>{t("title")}</PageHeaderTitle>
      </PageHeader>

      <WorkspaceSettingsForm
        error={error}
        isSubmitting={isSubmitting}
        workspace={workspace}
        onCancel={onCancel}
        onSubmit={onSubmit}
      />
    </>
  )
}

const useStyles = makeStyles(() => ({
  pageHeader: {
    paddingTop: 0,
  },
}))
