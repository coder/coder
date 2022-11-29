import { makeStyles } from "@material-ui/core/styles"
import { useActor } from "@xstate/react"
import { Loader } from "components/Loader/Loader"
import { FC, Suspense, useContext } from "react"
import { XServiceContext } from "../../xServices/StateContext"
import { Footer } from "../Footer/Footer"
import { Navbar } from "../Navbar/Navbar"
import { RequireAuth } from "../RequireAuth/RequireAuth"
import { UpdateCheckBanner } from "components/UpdateCheckBanner/UpdateCheckBanner"

interface AuthAndFrameProps {
  children: JSX.Element
}

/**
 * Wraps page in RequireAuth and renders it between Navbar and Footer
 */
export const AuthAndFrame: FC<AuthAndFrameProps> = ({ children }) => {
  const styles = useStyles()
  const xServices = useContext(XServiceContext)
  const [buildInfoState] = useActor(xServices.buildInfoXService)
  const [updateCheckState] = useActor(xServices.updateCheckXService)

  return (
    <RequireAuth>
      <div className={styles.site}>
        <Navbar />
        <UpdateCheckBanner updateCheck={updateCheckState.context.updateCheck} />
        <div className={styles.siteContent}>
          <Suspense fallback={<Loader />}>{children}</Suspense>
        </div>
        <Footer buildInfo={buildInfoState.context.buildInfo} />
      </div>
    </RequireAuth>
  )
}

const useStyles = makeStyles(() => ({
  site: {
    display: "flex",
    minHeight: "100vh",
    flexDirection: "column",
  },
  siteContent: {
    flex: 1,
  },
}))
