import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import SettingsOutlined from "@material-ui/icons/SettingsOutlined"
import { useMachine } from "@xstate/react"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { useOrganizationId } from "hooks/useOrganizationId"
import { createContext, FC, Suspense, useContext } from "react"
import {
  Link as RouterLink,
  NavLink,
  Outlet,
  useParams,
} from "react-router-dom"
import { combineClasses } from "util/combineClasses"
import {
  TemplateContext,
  templateMachine,
} from "xServices/template/templateXService"
import { Margins } from "components/Margins/Margins"
import { Stack } from "components/Stack/Stack"
import { Permissions } from "xServices/auth/authXService"
import { Loader } from "components/Loader/Loader"
import { usePermissions } from "hooks/usePermissions"
import { Avatar } from "components/Avatar/Avatar"

const Language = {
  settingsButton: "Settings",
  editButton: "Edit",
  createButton: "Create workspace",
  noDescription: "",
}

const useTemplateName = () => {
  const { template } = useParams()

  if (!template) {
    throw new Error("No template found in the URL")
  }

  return template
}

type TemplateLayoutContextValue = {
  context: TemplateContext
  permissions?: Permissions
}

const TemplateLayoutContext = createContext<
  TemplateLayoutContextValue | undefined
>(undefined)

export const useTemplateLayoutContext = (): TemplateLayoutContextValue => {
  const context = useContext(TemplateLayoutContext)
  if (!context) {
    throw new Error(
      "useTemplateLayoutContext only can be used inside of TemplateLayout",
    )
  }
  return context
}

const TemplateSettingsButton: FC<{ templateName: string }> = ({
  templateName,
}) => (
  <Link
    underline="none"
    component={RouterLink}
    to={`/templates/${templateName}/settings`}
  >
    <Button variant="outlined" startIcon={<SettingsOutlined />}>
      {Language.settingsButton}
    </Button>
  </Link>
)

const CreateWorkspaceButton: FC<{
  templateName: string
  className?: string
}> = ({ templateName, className }) => (
  <Link
    underline="none"
    component={RouterLink}
    to={`/templates/${templateName}/workspace`}
  >
    <Button className={className ?? ""} startIcon={<AddCircleOutline />}>
      {Language.createButton}
    </Button>
  </Link>
)

export const TemplateLayout: FC<{ children?: JSX.Element }> = ({
  children = <Outlet />,
}) => {
  const styles = useStyles()
  const organizationId = useOrganizationId()
  const templateName = useTemplateName()
  const [templateState, _] = useMachine(templateMachine, {
    context: {
      templateName,
      organizationId,
    },
  })
  const { template, permissions: templatePermissions } = templateState.context
  const permissions = usePermissions()
  const hasIcon = template && template.icon && template.icon !== ""

  if (!template) {
    return <Loader />
  }

  const generatePageHeaderActions = (): JSX.Element[] => {
    const pageActions: JSX.Element[] = []

    if (templatePermissions?.canUpdateTemplate) {
      pageActions.push(<TemplateSettingsButton templateName={template.name} />)
    }

    pageActions.push(<CreateWorkspaceButton templateName={template.name} />)

    return pageActions
  }

  return (
    <>
      <Margins>
        <PageHeader
          actions={
            <>
              {generatePageHeaderActions().map((action, i) => (
                <div key={i}>{action}</div>
              ))}
            </>
          }
        >
          <Stack direction="row" spacing={3} className={styles.pageTitle}>
            {hasIcon ? (
              <Avatar size="xl" src={template.icon} variant="square" fitImage />
            ) : (
              <Avatar size="xl">{template.name}</Avatar>
            )}

            <div>
              <PageHeaderTitle>
                {template.display_name.length > 0
                  ? template.display_name
                  : template.name}
              </PageHeaderTitle>
              <PageHeaderSubtitle condensed>
                {template.description === ""
                  ? Language.noDescription
                  : template.description}
              </PageHeaderSubtitle>
            </div>
          </Stack>
        </PageHeader>
      </Margins>

      <div className={styles.tabs}>
        <Margins>
          <Stack direction="row" spacing={0.25}>
            <NavLink
              end
              to={`/templates/${template.name}`}
              className={({ isActive }) =>
                combineClasses([
                  styles.tabItem,
                  isActive ? styles.tabItemActive : undefined,
                ])
              }
            >
              Summary
            </NavLink>
            <NavLink
              to={`/templates/${template.name}/permissions`}
              className={({ isActive }) =>
                combineClasses([
                  styles.tabItem,
                  isActive ? styles.tabItemActive : undefined,
                ])
              }
            >
              Permissions
            </NavLink>
          </Stack>
        </Margins>
      </div>

      <Margins>
        <TemplateLayoutContext.Provider
          value={{ permissions, context: templateState.context }}
        >
          <Suspense fallback={<Loader />}>{children}</Suspense>
        </TemplateLayoutContext.Provider>
      </Margins>
    </>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    pageTitle: {
      alignItems: "center",
    },
    iconWrapper: {
      width: theme.spacing(6),
      height: theme.spacing(6),
      "& img": {
        width: "100%",
      },
    },

    tabs: {
      borderBottom: `1px solid ${theme.palette.divider}`,
      marginBottom: theme.spacing(5),
    },

    tabItem: {
      textDecoration: "none",
      color: theme.palette.text.secondary,
      fontSize: 14,
      display: "block",
      padding: theme.spacing(0, 2, 2),

      "&:hover": {
        color: theme.palette.text.primary,
      },
    },

    tabItemActive: {
      color: theme.palette.text.primary,
      position: "relative",

      "&:before": {
        content: `""`,
        left: 0,
        bottom: 0,
        height: 2,
        width: "100%",
        background: theme.palette.secondary.dark,
        position: "absolute",
      },
    },
  }
})
