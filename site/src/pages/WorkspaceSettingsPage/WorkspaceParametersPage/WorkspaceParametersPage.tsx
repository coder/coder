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
import { FormValues, WorkspaceParametersForm } from "./WorkspaceParametersForm"
import { useNavigate } from "react-router-dom"
import { makeStyles } from "@material-ui/core/styles"
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { displaySuccess } from "components/GlobalSnackbar/utils"

const getWorkspaceParameters = async (workspace: Workspace) => {
  const latestBuild = workspace.latest_build
  const [templateVersionRichParameters, buildParameters] = await Promise.all([
    getTemplateVersionRichParameters(latestBuild.template_version_id),
    getWorkspaceBuildParameters(latestBuild.id),
  ])
  return {
    workspace,
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
  const styles = useStyles()
  const mutation = useMutation({
    mutationFn: (formValues: FormValues) =>
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

      <PageHeader className={styles.pageHeader}>
        <PageHeaderTitle>Workspace parameters</PageHeaderTitle>
      </PageHeader>

      {query.data ? (
        <WorkspaceParametersForm
          buildParameters={query.data.buildParameters}
          templateVersionRichParameters={
            query.data.templateVersionRichParameters
          }
          error={mutation.error}
          isSubmitting={mutation.isLoading}
          onSubmit={mutation.mutate}
          onCancel={() => {
            navigate("../..")
          }}
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
