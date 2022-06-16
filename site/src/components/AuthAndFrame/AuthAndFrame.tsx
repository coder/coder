import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { Footer } from "../Footer/Footer"
import { Navbar } from "../Navbar/Navbar"
import { RequireAuth } from "../RequireAuth/RequireAuth"

interface AuthAndFrameProps {
  children: JSX.Element
}

/**
 * Wraps page in RequireAuth and renders it between Navbar and Footer
 */
export const AuthAndFrame: FC<AuthAndFrameProps> = ({ children }) => {
  const styles = useStyles()

  return (
    <RequireAuth>
      <div className={styles.site}>
        <Navbar />
        <div className={styles.siteContent}>{children}</div>
        <Footer />
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
