import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import RefreshOutlined from "@material-ui/icons/RefreshOutlined"
import { BuildInfoResponse } from "api/typesGenerated"
import { CoderIcon } from "components/Icons/CoderIcon"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { Stack } from "components/Stack/Stack"
import { FC, useEffect, useState } from "react"
import { Helmet } from "react-helmet-async"
import { Margins } from "../Margins/Margins"

const fetchDynamicallyImportedModuleError =
  "Failed to fetch dynamically imported module" as const

export const RuntimeErrorState: FC<{ error: Error }> = ({ error }) => {
  const styles = useStyles()
  const [shouldDisplayMessage, setShouldDisplayMessage] = useState(false)

  // We use an effect to show a loading state if the page is trying to reload
  useEffect(() => {
    const isImportError = error.message.includes(
      fetchDynamicallyImportedModuleError,
    )
    const isRetried = window.location.search.includes("retries=1")

    if (isImportError && !isRetried) {
      const url = new URL(location.href)
      // Add a retry to avoid loops
      url.searchParams.set("retries", "1")
      location.assign(url.search)
      return
    }

    setShouldDisplayMessage(true)
  }, [error.message])

  return (
    <>
      <Helmet>
        <title>Something went wrong...</title>
      </Helmet>
      {shouldDisplayMessage ? (
        <Margins size="small" className={styles.root}>
          <div>
            <CoderIcon className={styles.logo} />
            <h1 className={styles.title}>Something went wrong...</h1>
            <p className={styles.text}>
              Please try reloading the page, if that doesn&lsquo;t work, you can
              ask for help in the{" "}
              <Link href="https://discord.gg/coder">
                Coder Discord community
              </Link>{" "}
              or{" "}
              <Link
                href={`https://github.com/coder/coder/issues/new?body=${encodeURIComponent(
                  [
                    ["**Version**", getStaticBuildInfo()].join("\n"),
                    ["**Path**", "`" + location.pathname + "`"].join("\n"),
                    ["**Error**", "```\n" + error.stack + "\n```"].join("\n"),
                  ].join("\n\n"),
                )}`}
                target="_blank"
              >
                open an issue
              </Link>
              .
            </p>
            <Stack direction="row" justifyContent="center">
              <Button
                startIcon={<RefreshOutlined />}
                onClick={() => {
                  window.location.reload()
                }}
              >
                Reload page
              </Button>
              <Button component="a" href="/" variant="outlined">
                Go to dashboard
              </Button>
            </Stack>
          </div>
        </Margins>
      ) : (
        <FullScreenLoader />
      )}
    </>
  )
}

// During the build process, we inject the build info into the HTML
const getStaticBuildInfo = () => {
  const buildInfoJson = document
    .querySelector("meta[property=build-info]")
    ?.getAttribute("content")

  if (buildInfoJson) {
    try {
      return JSON.parse(buildInfoJson) as BuildInfoResponse
    } catch {
      return "-- Set the version --"
    }
  }
}

const useStyles = makeStyles((theme) => ({
  root: {
    paddingTop: theme.spacing(4),
    paddingBottom: theme.spacing(4),
    textAlign: "center",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    minHeight: "100vh",
  },

  logo: {
    fontSize: theme.spacing(8),
  },

  title: {
    fontSize: theme.spacing(4),
    fontWeight: 400,
  },

  text: {
    fontSize: 16,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    marginBottom: theme.spacing(4),
  },
}))
