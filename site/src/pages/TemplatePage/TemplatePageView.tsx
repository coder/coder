import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import frontMatter from "front-matter"
import { FC } from "react"
import ReactMarkdown from "react-markdown"
import { Link as RouterLink } from "react-router-dom"
import { Template, TemplateVersion, WorkspaceResource } from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { TemplateResourcesTable } from "../../components/TemplateResourcesTable/TemplateResourcesTable"
import { TemplateStats } from "../../components/TemplateStats/TemplateStats"
import { WorkspaceSection } from "../../components/WorkspaceSection/WorkspaceSection"

const Language = {
  createButton: "Create workspace",
  noDescription: "No description",
  readmeTitle: "README",
  resourcesTitle: "Resources",
}

export interface TemplatePageViewProps {
  template: Template
  activeTemplateVersion: TemplateVersion
  templateResources: WorkspaceResource[]
}

export const TemplatePageView: FC<TemplatePageViewProps> = ({ template, activeTemplateVersion, templateResources }) => {
  const styles = useStyles()
  const readme = frontMatter(activeTemplateVersion.readme)

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter((resource) => resource.workspace_transition === "start")
  }

  return (
    <Margins>
      <PageHeader
        actions={
          <Link underline="none" component={RouterLink} to={`/templates/${template.name}/workspace`}>
            <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>
          </Link>
        }
      >
        <PageHeaderTitle>{template.name}</PageHeaderTitle>
        <PageHeaderSubtitle>
          {template.description === "" ? Language.noDescription : template.description}
        </PageHeaderSubtitle>
      </PageHeader>

      <Stack spacing={3}>
        <TemplateStats template={template} activeVersion={activeTemplateVersion} />
        <WorkspaceSection title={Language.resourcesTitle} contentsProps={{ className: styles.resourcesTableContents }}>
          <TemplateResourcesTable resources={getStartedResources(templateResources)} />
        </WorkspaceSection>
        <WorkspaceSection title={Language.readmeTitle} contentsProps={{ className: styles.readmeContents }}>
          <div className={styles.markdownWrapper}>
            <ReactMarkdown
              components={{
                a: ({ href, target, children }) => (
                  <Link href={href} target={target}>
                    {children}
                  </Link>
                ),
              }}
            >
              {readme.body}
            </ReactMarkdown>
          </div>
        </WorkspaceSection>
      </Stack>
    </Margins>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    readmeContents: {
      margin: 0,
    },
    markdownWrapper: {
      background: theme.palette.background.paper,
      padding: theme.spacing(3.5),
    },
    resourcesTableContents: {
      margin: 0,
    },
  }
})
