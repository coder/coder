import { makeStyles } from "@material-ui/core/styles"
import { useActor } from "@xstate/react"
import { Loader } from "components/Loader/Loader"
import { FC, Suspense, useContext, useEffect } from "react"
import { XServiceContext } from "../../xServices/StateContext"
import { Navbar } from "../Navbar/Navbar"
import { RequireAuth } from "../RequireAuth/RequireAuth"
import { UpdateCheckBanner } from "components/UpdateCheckBanner/UpdateCheckBanner"
import { Margins } from "components/Margins/Margins"

interface AuthAndFrameProps {
  children: JSX.Element
}

/**
 * Wraps page in RequireAuth and renders it between Navbar and Footer
 */
export const AuthAndFrame: FC<AuthAndFrameProps> = ({ children }) => {
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
    <RequireAuth>
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
          <Suspense fallback={<Loader />}>{children}</Suspense>
        </div>
      </div>
    </RequireAuth>
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
  },
}))
