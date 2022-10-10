import Avatar from "@material-ui/core/Avatar"
import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import SettingsOutlined from "@material-ui/icons/SettingsOutlined"
import { DeleteButton } from "components/DropdownButton/ActionCtas"
import { DropdownButton } from "components/DropdownButton/DropdownButton"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Markdown } from "components/Markdown/Markdown"
import frontMatter from "front-matter"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { firstLetter } from "util/firstLetter"
import {
  Template,
  TemplateDAUsResponse,
  TemplateVersion,
  WorkspaceResource,
} from "../../api/typesGenerated"
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
import { DAUChart } from "./DAUChart"

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
  templateDAUs?: TemplateDAUsResponse
  handleDeleteTemplate: (templateId: string) => void
  deleteTemplateError: Error | unknown
  canDeleteTemplate: boolean
}

export const TemplatePageView: FC<React.PropsWithChildren<TemplatePageViewProps>> = ({
  template,
  activeTemplateVersion,
  templateResources,
  templateVersions,
  templateDAUs,
  handleDeleteTemplate,
  deleteTemplateError,
  canDeleteTemplate,
}) => {
  const styles = useStyles()
  const readme = frontMatter(activeTemplateVersion.readme)
  const hasIcon = template.icon && template.icon !== ""

  const deleteError = Boolean(deleteTemplateError) && (
    <AlertBanner severity="error" error={deleteTemplateError} dismissible />
  )

  const getStartedResources = (resources: WorkspaceResource[]) => {
    return resources.filter((resource) => resource.workspace_transition === "start")
  }

  const createWorkspaceButton = (className?: string) => (
    <Link underline="none" component={RouterLink} to={`/templates/${template.name}/workspace`}>
      <Button className={className ?? ""} startIcon={<AddCircleOutline />}>
        {Language.createButton}
      </Button>
    </Link>
  )

  return (
    <Margins>
      <>
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

              {canDeleteTemplate ? (
                <DropdownButton
                  primaryAction={createWorkspaceButton(styles.actionButton)}
                  secondaryActions={[
                    {
                      action: "delete",
                      button: (
                        <DeleteButton handleAction={() => handleDeleteTemplate(template.id)} />
                      ),
                    },
                  ]}
                  canCancel={false}
                />
              ) : (
                createWorkspaceButton()
              )}
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
              <PageHeaderSubtitle condensed>
                {template.description === "" ? Language.noDescription : template.description}
              </PageHeaderSubtitle>
            </div>
          </Stack>
        </PageHeader>

        <Stack spacing={2.5}>
          {deleteError}
          {templateDAUs && <DAUChart templateDAUs={templateDAUs} />}
          <TemplateStats template={template} activeVersion={activeTemplateVersion} />
          <TemplateResourcesTable resources={getStartedResources(templateResources)} />
          <WorkspaceSection
            title={Language.readmeTitle}
            contentsProps={{ className: styles.readmeContents }}
          >
            <div className={styles.markdownWrapper}>
              <Markdown>{readme.body}</Markdown>
            </div>
          </WorkspaceSection>
          <WorkspaceSection
            title={Language.versionsTitle}
            contentsProps={{ className: styles.versionsTableContents }}
          >
            <VersionsTable versions={templateVersions} />
          </WorkspaceSection>
        </Stack>
      </>
    </Margins>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    actionButton: {
      border: "none",
      borderRadius: `${theme.shape.borderRadius}px 0px 0px ${theme.shape.borderRadius}px`,
    },
    readmeContents: {
      margin: 0,
    },
    markdownWrapper: {
      background: theme.palette.background.paper,
      padding: theme.spacing(3, 4),
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
