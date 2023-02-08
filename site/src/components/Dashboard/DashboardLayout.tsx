import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import { Loader } from "components/Loader/Loader"
import { FC, Suspense } from "react"
import { Navbar } from "../Navbar/Navbar"
import { UpdateCheckBanner } from "components/UpdateCheckBanner/UpdateCheckBanner"
import { Margins } from "components/Margins/Margins"
import { Outlet } from "react-router-dom"
import { LicenseBanner } from "components/LicenseBanner/LicenseBanner"
import { ServiceBanner } from "components/ServiceBanner/ServiceBanner"
import { updateCheckMachine } from "xServices/updateCheck/updateCheckXService"
import { usePermissions } from "hooks/usePermissions"
import { UpdateCheckResponse } from "api/typesGenerated"
import { DashboardProvider } from "./DashboardProvider"
import { dashboardContentBottomPadding } from "theme/constants"

export const DashboardLayout: FC = () => {
  const styles = useStyles()
  const permissions = usePermissions()
  const [updateCheckState, updateCheckSend] = useMachine(updateCheckMachine, {
    context: {
      permissions,
    },
  })
  const { error: updateCheckError, updateCheck } = updateCheckState.context

  return (
    <DashboardProvider>
      <ServiceBanner />
      <LicenseBanner />

      <div className={styles.site}>
        <Navbar />

        {updateCheckState.matches("show") && (
          <div className={styles.updateCheckBanner}>
            <Margins>
              <UpdateCheckBanner
                // We can trust when it is show, the update check is filled
                // unfortunately, XState does not has typed state - context yet
                updateCheck={updateCheck as UpdateCheckResponse}
                error={updateCheckError}
                onDismiss={() => updateCheckSend("DISMISS")}
              />
            </Margins>
          </div>
        )}

        <div className={styles.siteContent}>
          <Suspense fallback={<Loader />}>
            <Outlet />
          </Suspense>
        </div>
      </div>
    </DashboardProvider>
  )
}

const useStyles = makeStyles((theme) => ({
  site: {
    display: "flex",
    minHeight: "100vh",
    flexDirection: "column",
  },
  updateCheckBanner: {
    // Add spacing at the top and remove some from the bottom. Removal
    // is necessary to avoid a visual jerk when the banner is dismissed.
    // It also give a more pleasant distance to the site content when
    // the banner is visible.
    marginTop: theme.spacing(2),
    marginBottom: -theme.spacing(2),
  },
  siteContent: {
    flex: 1,
    paddingBottom: dashboardContentBottomPadding, // Add bottom space since we don't use a footer
  },
}))
