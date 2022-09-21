import { makeStyles } from "@material-ui/core/styles"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"

export interface TemplateCollaboratorsPageViewProps {
  deleteTemplateError: Error | unknown
}

export const TemplateCollaboratorsPageView: FC<
  React.PropsWithChildren<TemplateCollaboratorsPageViewProps>
> = ({ deleteTemplateError }) => {
  const deleteError = deleteTemplateError ? (
    <ErrorSummary error={deleteTemplateError} dismissible />
  ) : null

  return (
    <Stack spacing={2.5}>
      {deleteError}
      <h2>Collaborators</h2>
    </Stack>
  )
}

export const useStyles = makeStyles(() => {
  return {}
})
