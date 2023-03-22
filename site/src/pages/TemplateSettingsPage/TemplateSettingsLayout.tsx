import { makeStyles } from "@material-ui/core/styles"
import { Sidebar } from "./Sidebar"
import { Stack } from "components/Stack/Stack"
import { createContext, FC, Suspense, useContext } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "../../util/page"
import { Loader } from "components/Loader/Loader"
import { Outlet, useParams } from "react-router-dom"
import { Margins } from "components/Margins/Margins"
import { getTemplateByName } from "api/api"
import { useQuery } from "@tanstack/react-query"
import { useOrganizationId } from "hooks/useOrganizationId"

const fetchTemplate = (orgId: string, name: string) => {
  return getTemplateByName(orgId, name)
}

const useTemplate = (orgId: string, name: string) => {
  return useQuery({
    queryKey: ["template", orgId, name],
    queryFn: () => fetchTemplate(orgId, name),
  })
}

const TemplateSettingsContext = createContext<
  Awaited<ReturnType<typeof fetchTemplate>> | undefined
>(undefined)

export const useTemplateSettingsContext = () => {
  const context = useContext(TemplateSettingsContext)

  if (!context) {
    throw new Error(
      "useTemplateSettingsContext must be used within a TemplateSettingsContext.Provider",
    )
  }

  return context
}

export const TemplateSettingsLayout: FC = () => {
  const styles = useStyles()
  const orgId = useOrganizationId()
  const { template: templateName } = useParams() as { template: string }
  const { data: template } = useTemplate(orgId, templateName)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Settings")}</title>
      </Helmet>

      {template ? (
        <TemplateSettingsContext.Provider value={template}>
          <Margins>
            <Stack className={styles.wrapper} direction="row" spacing={10}>
              <Sidebar template={template} />
              <Suspense fallback={<Loader />}>
                <main className={styles.content}>
                  <Outlet />
                </main>
              </Suspense>
            </Stack>
          </Margins>
        </TemplateSettingsContext.Provider>
      ) : (
        <Loader />
      )}
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(6, 0),
  },

  content: {
    width: "100%",
  },
}))
