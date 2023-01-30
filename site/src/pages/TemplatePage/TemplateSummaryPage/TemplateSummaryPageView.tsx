import { makeStyles } from "@material-ui/core/styles"
import {
  Template,
  TemplateDAUsResponse,
  TemplateVersion,
  WorkspaceResource,
} from "api/typesGenerated"
import { MemoizedMarkdown } from "components/Markdown/Markdown"
import { Stack } from "components/Stack/Stack"
import { TemplateResourcesTable } from "components/TemplateResourcesTable/TemplateResourcesTable"
import { TemplateStats } from "components/TemplateStats/TemplateStats"
import { VersionsTable } from "components/VersionsTable/VersionsTable"
import frontMatter from "front-matter"
import { FC } from "react"
import { DAUChart } from "../../../components/DAUChart/DAUChart"

export interface TemplateSummaryPageViewProps {
  template: Template
  activeTemplateVersion: TemplateVersion
  templateResources: WorkspaceResource[]
  templateVersions?: TemplateVersion[]
  templateDAUs?: TemplateDAUsResponse
}

export const TemplateSummaryPageView: FC<
  React.PropsWithChildren<TemplateSummaryPageViewProps>
> = ({
  template,
  activeTemplateVersion,
  templateResources,
  templateVersions,
  templateDAUs,
}) => {
  const styles = useStyles()
  const readme = frontMatter(activeTemplateVersion.readme)

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter(
      (resource) => resource.workspace_transition === "start",
    )
  }

  return (
    <Stack spacing={4}>
      <TemplateStats
        template={template}
        activeVersion={activeTemplateVersion}
      />
      {templateDAUs && <DAUChart daus={templateDAUs} />}
      <TemplateResourcesTable
        resources={getStartedResources(templateResources)}
      />

      <div className={styles.markdownSection} id="readme">
        <div className={styles.readmeLabel}>README.md</div>
        <div className={styles.markdownWrapper}>
          <MemoizedMarkdown>{readme.body}</MemoizedMarkdown>
        </div>
      </div>

      <VersionsTable versions={templateVersions} />
    </Stack>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    markdownSection: {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      borderRadius: theme.shape.borderRadius,
    },

    readmeLabel: {
      color: theme.palette.text.secondary,
      fontWeight: 600,
      padding: theme.spacing(2, 3),
      borderBottom: `1px solid ${theme.palette.divider}`,
    },

    markdownWrapper: {
      padding: theme.spacing(0, 3, 5),
      maxWidth: 800,
      margin: "auto",
    },
  }
})
