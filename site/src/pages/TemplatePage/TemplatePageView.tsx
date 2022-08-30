import Avatar from "@material-ui/core/Avatar"
import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import SettingsOutlined from "@material-ui/icons/SettingsOutlined"
import frontMatter from "front-matter"
import { FC } from "react"
import ReactMarkdown from "react-markdown"
import { Link as RouterLink } from "react-router-dom"
import { firstLetter } from "util/firstLetter"
import { Template, TemplateVersion, WorkspaceResource } from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { TemplateResourcesTable } from "../../components/TemplateResourcesTable/TemplateResourcesTable"
import { TemplateStats } from "../../components/TemplateStats/TemplateStats"
import { VersionsTable } from "../../components/VersionsTable/VersionsTable"
import { WorkspaceSection } from "../../components/WorkspaceSection/WorkspaceSection"

const Language = {
  settingsButton: "Settings",
  createButton: "Create workspace",
  noDescription: "",
  readmeTitle: "README",
  resourcesTitle: "Resources",
  versionsTitle: "Version history",
}

export interface TemplatePageViewProps {
  template: Template
  activeTemplateVersion: TemplateVersion
  templateResources: WorkspaceResource[]
  templateVersions?: TemplateVersion[]
}

export const TemplatePageView: FC<React.PropsWithChildren<TemplatePageViewProps>> = ({
  template,
  activeTemplateVersion,
  templateResources,
  templateVersions,
}) => {
  const styles = useStyles()
  const readme = frontMatter(activeTemplateVersion.readme)
  const hasIcon = template.icon && template.icon !== ""

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter((resource) => resource.workspace_transition === "start")
  }

  return (
    <Margins>
      <PageHeader
        actions={
          <>
            <Link
              underline="none"
              component={RouterLink}
              to={`/templates/${template.name}/settings`}
            >
              <Button variant="outlined" startIcon={<SettingsOutlined />}>
                {Language.settingsButton}
              </Button>
            </Link>
            <Link
              underline="none"
              component={RouterLink}
              to={`/templates/${template.name}/workspace`}
            >
              <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>
            </Link>
          </>
        }
      >
        <Stack direction="row" spacing={3} className={styles.pageTitle}>
          <div>
            {hasIcon ? (
              <div className={styles.iconWrapper}>
                <img src={template.icon} alt="" />
              </div>
            ) : (
              <Avatar className={styles.avatar}>{firstLetter(template.name)}</Avatar>
            )}
          </div>
          <div>
            <PageHeaderTitle>{template.name}</PageHeaderTitle>
            <PageHeaderSubtitle>
              {template.description === "" ? Language.noDescription : template.description}
            </PageHeaderSubtitle>
          </div>
        </Stack>
      </PageHeader>

      <Stack spacing={2.5}>
        <TemplateStats template={template} activeVersion={activeTemplateVersion} />
        <WorkspaceSection
          title={Language.resourcesTitle}
          contentsProps={{ className: styles.resourcesTableContents }}
        >
          <TemplateResourcesTable resources={getStartedResources(templateResources)} />
        </WorkspaceSection>
        <WorkspaceSection
          title={Language.readmeTitle}
          contentsProps={{ className: styles.readmeContents }}
        >
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
        <WorkspaceSection
          title={Language.versionsTitle}
          contentsProps={{ className: styles.versionsTableContents }}
        >
          <VersionsTable versions={templateVersions} />
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

      // Adds text wrapping to <pre> tag added by ReactMarkdown
      "& pre": {
        whiteSpace: "pre-wrap",
        wordWrap: "break-word",
      },
    },
    resourcesTableContents: {
      margin: 0,
    },
    versionsTableContents: {
      margin: 0,
    },
    pageTitle: {
      alignItems: "center",
    },
    avatar: {
      width: theme.spacing(6),
      height: theme.spacing(6),
      fontSize: theme.spacing(3),
    },
    iconWrapper: {
      width: theme.spacing(6),
      height: theme.spacing(6),
      "& img": {
        width: "100%",
      },
    },
  }
})
