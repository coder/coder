import { makeStyles } from "@material-ui/core/styles"
import { Sidebar } from "./Sidebar"
import { Stack } from "components/Stack/Stack"
import { FC, PropsWithChildren, Suspense } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "../../util/page"
import { Margins } from "../Margins/Margins"
import { useMe } from "hooks/useMe"
import { Loader } from "components/Loader/Loader"

export const SettingsLayout: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  const me = useMe()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Settings")}</title>
      </Helmet>

      <Margins>
        <Stack className={styles.wrapper} direction="row" spacing={6}>
          <Sidebar user={me} />
          <Suspense fallback={<Loader />}>
            <main className={styles.content}>{children}</main>
          </Suspense>
        </Stack>
      </Margins>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(6, 0),
  },

  content: {
    maxWidth: 800,
    width: "100%",
  },
}))
