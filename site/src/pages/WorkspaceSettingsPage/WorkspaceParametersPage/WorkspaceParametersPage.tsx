import {
  getTemplateVersionRichParameters,
  getWorkspaceBuildParameters,
  postWorkspaceBuild,
} from "api/api"
import { Workspace } from "api/typesGenerated"
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
import { displaySuccess } from "components/GlobalSnackbar/utils"
import { FC } from "react"

const getWorkspaceParameters = async (workspace: Workspace) => {
  const latestBuild = workspace.latest_build
  const [templateVersionRichParameters, buildParameters] = await Promise.all([
    getTemplateVersionRichParameters(latestBuild.template_version_id),
    getWorkspaceBuildParameters(latestBuild.id),
  ])
  return {
    templateVersionRichParameters,
    buildParameters,
  }
}

const WorkspaceParametersPage = () => {
  const { workspace } = useWorkspaceSettingsContext()
  const query = useQuery({
    queryKey: ["workspaceSettings", workspace.id],
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
      displaySuccess(
        "Parameters updated successfully",
        "A new build was started to apply the new parameters",
      )
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
