import { makeStyles } from "@material-ui/core/styles"
import { useOrganizationId } from "hooks/useOrganizationId"
import { createContext, FC, Suspense, useContext } from "react"
import { NavLink, Outlet, useNavigate, useParams } from "react-router-dom"
import { combineClasses } from "util/combineClasses"
import { Margins } from "components/Margins/Margins"
import { Stack } from "components/Stack/Stack"
import { Loader } from "components/Loader/Loader"
import { TemplatePageHeader } from "./TemplatePageHeader"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import {
  checkAuthorization,
  getTemplateByName,
  getTemplateDAUs,
  getTemplateVersion,
  getTemplateVersionResources,
  getTemplateVersions,
} from "api/api"
import { useQuery } from "@tanstack/react-query"

const templatePermissions = (templateId: string) => ({
  canUpdateTemplate: {
    object: {
      resource_type: "template",
      resource_id: templateId,
    },
    action: "update",
  },
})

const fetchTemplate = async (orgId: string, templateName: string) => {
  const template = await getTemplateByName(orgId, templateName)
  const [activeVersion, resources, versions, daus, permissions] =
    await Promise.all([
      getTemplateVersion(template.active_version_id),
      getTemplateVersionResources(template.active_version_id),
      getTemplateVersions(template.id),
      getTemplateDAUs(template.id),
      checkAuthorization({
        checks: templatePermissions(template.id),
      }),
    ])

  return {
    template,
    activeVersion,
    resources,
    versions,
    daus,
    permissions,
  }
}

const useTemplateData = (orgId: string, templateName: string) => {
  return useQuery({
    queryKey: ["template", templateName],
    queryFn: () => fetchTemplate(orgId, templateName),
  })
}

type TemplateLayoutContextValue = Awaited<ReturnType<typeof fetchTemplate>>

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
  const orgId = useOrganizationId()
  const { template } = useParams() as { template: string }
  const templateData = useTemplateData(orgId, template)

  if (templateData.error) {
    return (
      <div className={styles.error}>
        <AlertBanner severity="error" error={templateData.error} />
      </div>
    )
  }

  if (templateData.isLoading) {
    return <Loader />
  }

  // Make typescript happy
  if (!templateData.data) {
    return <></>
  }

  return (
    <>
      <TemplatePageHeader
        template={templateData.data.template}
        permissions={templateData.data.permissions}
        onDeleteTemplate={() => {
          navigate("/templates")
        }}
      />

      <div className={styles.tabs}>
        <Margins>
          <Stack direction="row" spacing={0.25}>
            <NavLink
              end
              to={`/templates/${template}`}
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
              to={`/templates/${template}/permissions`}
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
        <TemplateLayoutContext.Provider value={templateData.data}>
          <Suspense fallback={<Loader />}>{children}</Suspense>
        </TemplateLayoutContext.Provider>
      </Margins>
    </>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    error: {
      margin: theme.spacing(2),
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
