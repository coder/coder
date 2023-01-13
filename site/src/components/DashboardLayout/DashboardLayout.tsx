import { makeStyles } from "@material-ui/core/styles"
import { useActor } from "@xstate/react"
import { Loader } from "components/Loader/Loader"
import { FC, Suspense, useContext, useEffect } from "react"
import { XServiceContext } from "../../xServices/StateContext"
import { Navbar } from "../Navbar/Navbar"
import { UpdateCheckBanner } from "components/UpdateCheckBanner/UpdateCheckBanner"
import { Margins } from "components/Margins/Margins"
import { Outlet } from "react-router-dom"

export const DashboardLayout: FC = () => {
  const styles = useStyles()
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const [updateCheckState, updateCheckSend] = useActor(
    xServices.updateCheckXService,
  )

  useEffect(() => {
    if (authState.matches("signedIn")) {
      updateCheckSend("CHECK")
    } else {
      updateCheckSend("CLEAR")
    }
  }, [authState, updateCheckSend])

  return (
    <div className={styles.site}>
      <Navbar />
      {updateCheckState.context.show && (
        <div className={styles.updateCheckBanner}>
          <Margins>
            <UpdateCheckBanner
              updateCheck={updateCheckState.context.updateCheck}
              error={updateCheckState.context.error}
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
    paddingBottom: theme.spacing(10),
  },
}))
