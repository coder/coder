import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { createContext, FC, Suspense, useContext } from "react"
import { NavLink, Outlet, useNavigate, useParams } from "react-router-dom"
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
import { TemplatePageHeader } from "./TemplatePageHeader"

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

export const TemplateLayout: FC<{ children?: JSX.Element }> = ({
  children = <Outlet />,
}) => {
  const navigate = useNavigate()
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

  if (!template || !templatePermissions) {
    return <Loader />
  }

  return (
    <>
      <TemplatePageHeader
        template={template}
        permissions={templatePermissions}
        onDeleteTemplate={() => {
          navigate("/templates")
        }}
      />

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
