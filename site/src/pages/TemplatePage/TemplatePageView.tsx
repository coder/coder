import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import frontMatter from "front-matter"
import React from "react"
import ReactMarkdown from "react-markdown"
import { Link as RouterLink } from "react-router-dom"
import { Template, TemplateVersion, WorkspaceResource } from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { TemplateResourcesTable } from "../../components/TemplateResourcesTable/TemplateResourcesTable"
import { TemplateStats } from "../../components/TemplateStats/TemplateStats"
import { WorkspaceSection } from "../../components/WorkspaceSection/WorkspaceSection"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"

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

export const TemplatePageView: React.FC<TemplatePageViewProps> = ({
  template,
  activeTemplateVersion,
  templateResources,
}) => {
  const styles = useStyles()
  const readme = frontMatter(activeTemplateVersion.readme)

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter((resource) => resource.workspace_transition === "start")
  }

  return (
    <Margins>
      <div className={styles.header}>
        <div>
          <Typography variant="h4" className={styles.title}>
            {template.name}
          </Typography>

          <Typography color="textSecondary" className={styles.subtitle}>
            {template.description === "" ? Language.noDescription : template.description}
          </Typography>
        </div>

        <div className={styles.headerActions}>
          <Link underline="none" component={RouterLink} to={`/workspaces/new?template=${template.name}`}>
            <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>
          </Link>
        </div>
      </div>

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
    root: {
      display: "flex",
      flexDirection: "column",
    },
    header: {
      paddingTop: theme.spacing(5),
      paddingBottom: theme.spacing(5),
      fontFamily: MONOSPACE_FONT_FAMILY,
      display: "flex",
      alignItems: "center",
    },
    headerActions: {
      marginLeft: "auto",
    },
    title: {
      fontWeight: 600,
      fontFamily: "inherit",
    },
    subtitle: {
      fontFamily: "inherit",
      marginTop: theme.spacing(0.5),
    },
    layout: {
      alignItems: "flex-start",
    },
    main: {
      width: "100%",
    },
    sidebar: {
      width: theme.spacing(32),
      flexShrink: 0,
    },
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
