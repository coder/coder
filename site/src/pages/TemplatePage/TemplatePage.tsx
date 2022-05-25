import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import { useMachine } from "@xstate/react"
import React from "react"
import ReactMarkdown from "react-markdown"
import { Link as RouterLink, useParams } from "react-router-dom"
import { WorkspaceResource } from "../../api/typesGenerated"
import { Loader } from "../../components/Loader/Loader"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { TemplateResourcesTable } from "../../components/TemplateResourcesTable/TemplateResourcesTable"
import { TemplateStats } from "../../components/TemplateStats/TemplateStats"
import { WorkspaceSection } from "../../components/WorkspaceSection/WorkspaceSection"
import { useOrganizationID } from "../../hooks/useOrganizationID"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { templateMachine } from "../../xServices/template/templateXService"

const Language = {
  createButton: "Create workspace",
}

const useTemplateName = () => {
  const { template } = useParams()

  if (!template) {
    throw new Error("No template found in the URL")
  }

  return template
}

export const TemplatePage: React.FC = () => {
  const organizationId = useOrganizationID()
  const templateName = useTemplateName()
  const [templateState] = useMachine(templateMachine, {
    context: {
      templateName,
      organizationId,
    },
  })
  const { template, activeTemplateVersion, templateResources } = templateState.context
  const isLoading = !template || !activeTemplateVersion || !templateResources
  const styles = useStyles()

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter((resource) => resource.workspace_transition === "start")
  }

  if (isLoading) {
    return <Loader />
  }

  return (
    <Margins>
      <div className={styles.header}>
        <div>
          <Typography variant="h4" className={styles.title}>
            {template.name}
          </Typography>

          <Typography color="textSecondary" className={styles.subtitle}>
            {template.description === "" ? "No description" : template.description}
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
        <WorkspaceSection title="Resources" contentsProps={{ className: styles.resourcesTableContents }}>
          <TemplateResourcesTable resources={getStartedResources(templateResources)} />
        </WorkspaceSection>
        <WorkspaceSection title="README" contentsProps={{ className: styles.readmeContents }}>
          <div className={styles.markdownWrapper}>
            <ReactMarkdown>{activeTemplateVersion.readme}</ReactMarkdown>
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
      whiteSpace: "pre-wrap",
      background: theme.palette.background.paper,
      padding: theme.spacing(6),
      lineHeight: "180%",
    },
    resourcesTableContents: {
      margin: 0,
    },
  }
})
