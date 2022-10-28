import { makeStyles } from "@material-ui/core/styles"
import {
  Template,
  TemplateDAUsResponse,
  TemplateVersion,
  WorkspaceResource,
} from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Markdown } from "components/Markdown/Markdown"
import { Stack } from "components/Stack/Stack"
import { TemplateResourcesTable } from "components/TemplateResourcesTable/TemplateResourcesTable"
import { TemplateStats } from "components/TemplateStats/TemplateStats"
import { VersionsTable } from "components/VersionsTable/VersionsTable"
import { WorkspaceSection } from "components/WorkspaceSection/WorkspaceSection"
import frontMatter from "front-matter"
import { FC } from "react"
import { DAUChart } from "./DAUChart"

const Language = {
  readmeTitle: "README",
  resourcesTitle: "Resources",
}

export interface TemplateSummaryPageViewProps {
  template: Template
  activeTemplateVersion: TemplateVersion
  templateResources: WorkspaceResource[]
  templateVersions?: TemplateVersion[]
  templateDAUs?: TemplateDAUsResponse
  deleteTemplateError: Error | unknown
}

export const TemplateSummaryPageView: FC<
  React.PropsWithChildren<TemplateSummaryPageViewProps>
> = ({
  template,
  activeTemplateVersion,
  templateResources,
  templateVersions,
  templateDAUs,
  deleteTemplateError,
}) => {
  const styles = useStyles()
  const readme = frontMatter(activeTemplateVersion.readme)

  const deleteError = deleteTemplateError ? (
    <AlertBanner severity="error" error={deleteTemplateError} dismissible />
  ) : null

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter(
      (resource) => resource.workspace_transition === "start",
    )
  }

  return (
    <Stack spacing={4}>
      {deleteError}
      <TemplateStats
        template={template}
        activeVersion={activeTemplateVersion}
      />
      {templateDAUs && <DAUChart templateDAUs={templateDAUs} />}
      <TemplateResourcesTable
        resources={getStartedResources(templateResources)}
      />
      <WorkspaceSection
        title={Language.readmeTitle}
        contentsProps={{ className: styles.readmeContents }}
      >
        <div className={styles.markdownWrapper}>
          <Markdown>{readme.body}</Markdown>
        </div>
      </WorkspaceSection>
      <VersionsTable versions={templateVersions} />
    </Stack>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    readmeContents: {
      margin: 0,
    },
    markdownWrapper: {
      background: theme.palette.background.paper,
      padding: theme.spacing(3, 4),
    },
  }
})
