import {
  Template,
  TemplateVersion,
  WorkspaceResource,
} from "api/typesGenerated"
import { Loader } from "components/Loader/Loader"
import { Stack } from "components/Stack/Stack"
import { TemplateResourcesTable } from "components/TemplateResourcesTable/TemplateResourcesTable"
import { TemplateStats } from "components/TemplateStats/TemplateStats"
import { FC, useEffect } from "react"
import { DAUChart } from "../../../components/DAUChart/DAUChart"
import { TemplateSummaryData } from "./data"
import { useLocation, useNavigate } from "react-router-dom"
import { Maybe } from "components/Conditionals/Maybe"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import Link from "@material-ui/core/Link"

export interface TemplateSummaryPageViewProps {
  data?: TemplateSummaryData
  template: Template
  activeVersion: TemplateVersion
}

export const TemplateSummaryPageView: FC<TemplateSummaryPageViewProps> = ({
  data,
  template,
  activeVersion,
}) => {
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    if (location.hash === "#readme") {
      // We moved the readme to the docs page, but we known that some users
      // have bookmarked the readme or linked it elsewhere. Redirect them to the docs page.
      navigate(`/templates/${template.name}/docs`, { replace: true })
    }
  }, [template, navigate, location])

  if (!data) {
    return <Loader />
  }

  const { daus, resources } = data

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter(
      (resource) => resource.workspace_transition === "start",
    )
  }

  return (
    <Stack spacing={4}>
      {activeVersion.warnings && (
        <Maybe
          condition={Boolean(
            activeVersion.warnings.includes("DEPRECATED_PARAMETERS"),
          )}
        >
          <AlertBanner severity="warning">
            <div>
              This template uses legacy parameters which will be deprecated in
              the next Coder release. Learn how to migrate in{" "}
              <Link href="https://coder.com/docs/v2/latest/templates/parameters#migration">
                our documentation
              </Link>
              .
            </div>
          </AlertBanner>
        </Maybe>
      )}
      <TemplateStats template={template} activeVersion={activeVersion} />
      {daus && <DAUChart daus={daus} />}
      <TemplateResourcesTable resources={getStartedResources(resources)} />
    </Stack>
  )
}
