import { getWorkspaceParameters, postWorkspaceBuild } from "api/api"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { useWorkspaceSettingsContext } from "../WorkspaceSettingsLayout"
import { useMutation, useQuery } from "@tanstack/react-query"
import { Loader } from "components/Loader/Loader"
import {
  WorkspaceParametersFormValues,
  WorkspaceParametersForm,
} from "./WorkspaceParametersForm"
import { useNavigate } from "react-router-dom"
import { makeStyles } from "@mui/styles"
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { FC } from "react"
import { isApiValidationError } from "api/errors"
import { ErrorAlert } from "components/Alert/ErrorAlert"

const WorkspaceParametersPage = () => {
  const { workspace } = useWorkspaceSettingsContext()
  const query = useQuery({
    queryKey: ["workspace", workspace.id, "parameters"],
    queryFn: () => getWorkspaceParameters(workspace),
  })
  const navigate = useNavigate()
  const mutation = useMutation({
    mutationFn: (formValues: WorkspaceParametersFormValues) =>
      postWorkspaceBuild(workspace.id, {
        transition: "start",
        rich_parameter_values: formValues.rich_parameter_values,
      }),
    onSuccess: () => {
      navigate(`/${workspace.owner_name}/${workspace.name}`)
    },
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle([workspace.name, "Parameters"])}</title>
      </Helmet>

      <WorkspaceParametersPageView
        data={query.data}
        submitError={mutation.error}
        isSubmitting={mutation.isLoading}
        onSubmit={mutation.mutate}
        onCancel={() => {
          navigate("../..")
        }}
      />
    </>
  )
}

export type WorkspaceParametersPageViewProps = {
  data: Awaited<ReturnType<typeof getWorkspaceParameters>> | undefined
  submitError: unknown
  isSubmitting: boolean
  onSubmit: (formValues: WorkspaceParametersFormValues) => void
  onCancel: () => void
}

export const WorkspaceParametersPageView: FC<
  WorkspaceParametersPageViewProps
> = ({ data, submitError, isSubmitting, onSubmit, onCancel }) => {
  const styles = useStyles()

  return (
    <>
      <PageHeader className={styles.pageHeader}>
        <PageHeaderTitle>Workspace parameters</PageHeaderTitle>
      </PageHeader>

      {submitError && !isApiValidationError(submitError) && (
        <ErrorAlert error={submitError} sx={{ mb: 6 }} />
      )}

      {data ? (
        <WorkspaceParametersForm
          buildParameters={data.buildParameters}
          templateVersionRichParameters={data.templateVersionRichParameters}
          error={submitError}
          isSubmitting={isSubmitting}
          onSubmit={onSubmit}
          onCancel={onCancel}
        />
      ) : (
        <Loader />
      )}
    </>
  )
}

const useStyles = makeStyles(() => ({
  pageHeader: {
    paddingTop: 0,
  },
}))

export default WorkspaceParametersPage
