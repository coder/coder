import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { ComponentProps, FC } from "react"
import { TemplateScheduleForm } from "./TemplateScheduleForm"
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { makeStyles } from "@material-ui/core/styles"

export interface TemplateSchedulePageViewProps {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  submitError?: unknown
  initialTouched?: ComponentProps<typeof TemplateScheduleForm>["initialTouched"]
  canSetMaxTTL: boolean
}

export const TemplateSchedulePageView: FC<TemplateSchedulePageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  isSubmitting,
  canSetMaxTTL,
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
        canSetMaxTTL={canSetMaxTTL}
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
