import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { ComponentProps, FC } from "react"
import { TemplateScheduleForm } from "./TemplateScheduleForm"
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { makeStyles } from "@mui/styles"

export interface TemplateSchedulePageViewProps {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  submitError?: unknown
  initialTouched?: ComponentProps<typeof TemplateScheduleForm>["initialTouched"]
  allowAdvancedScheduling: boolean
  allowWorkspaceActions: boolean
}

export const TemplateSchedulePageView: FC<TemplateSchedulePageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  isSubmitting,
  allowAdvancedScheduling,
  allowWorkspaceActions,
  submitError,
  initialTouched,
}) => {
  const styles = useStyles()

  return (
    <>
      <PageHeader className={styles.pageHeader}>
        <PageHeaderTitle>Template schedule</PageHeaderTitle>
      </PageHeader>

      <TemplateScheduleForm
        allowAdvancedScheduling={allowAdvancedScheduling}
        allowWorkspaceActions={allowWorkspaceActions}
        initialTouched={initialTouched}
        isSubmitting={isSubmitting}
        template={template}
        onSubmit={onSubmit}
        onCancel={onCancel}
        error={submitError}
      />
    </>
  )
}

const useStyles = makeStyles(() => ({
  pageHeader: {
    paddingTop: 0,
  },
}))
