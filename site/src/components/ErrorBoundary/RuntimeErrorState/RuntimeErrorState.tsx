import Button from "@mui/material/Button"
import Link from "@mui/material/Link"
import { makeStyles } from "@mui/styles"
import RefreshOutlined from "@mui/icons-material/RefreshOutlined"
import { BuildInfoResponse } from "api/typesGenerated"
import { CopyButton } from "components/CopyButton/CopyButton"
import { CoderIcon } from "components/Icons/CoderIcon"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { Stack } from "components/Stack/Stack"
import { FC, useEffect, useState } from "react"
import { Helmet } from "react-helmet-async"
import { Margins } from "../../Margins/Margins"

const fetchDynamicallyImportedModuleError =
  "Failed to fetch dynamically imported module"

export type RuntimeErrorStateProps = { error: Error }

export const RuntimeErrorState: FC<RuntimeErrorStateProps> = ({ error }) => {
  const styles = useStyles()
  const [checkingError, setCheckingError] = useState(true)
  const [staticBuildInfo, setStaticBuildInfo] = useState<BuildInfoResponse>()
  const coderVersion = staticBuildInfo?.version

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

    setCheckingError(false)
  }, [error.message])

  useEffect(() => {
    if (!checkingError) {
      setStaticBuildInfo(getStaticBuildInfo())
    }
  }, [checkingError])

  return (
    <>
      <Helmet>
        <title>Something went wrong...</title>
      </Helmet>
      {!checkingError ? (
        <Margins className={styles.root}>
          <div className={styles.innerRoot}>
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
                    ["**Version**", coderVersion ?? "-- Set version --"].join(
                      "\n",
                    ),
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
              <Button component="a" href="/">
                Go to dashboard
              </Button>
            </Stack>
            {error.stack && (
              <div className={styles.stack}>
                <div className={styles.stackHeader}>
                  Stacktrace
                  <CopyButton
                    buttonClassName={styles.copyButton}
                    text={error.stack}
                    tooltipTitle="Copy stacktrace"
                  />
                </div>
                <pre className={styles.stackCode}>{error.stack}</pre>
              </div>
            )}
            {coderVersion && (
              <div className={styles.version}>Version: {coderVersion}</div>
            )}
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
      return undefined
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
    minHeight: "100%",
    maxWidth: theme.spacing(75),
  },

  innerRoot: { width: "100%" },

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

  stack: {
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 4,
    marginTop: theme.spacing(8),
    display: "block",
    textAlign: "left",
  },

  stackHeader: {
    fontSize: 10,
    textTransform: "uppercase",
    fontWeight: 600,
    letterSpacing: 1,
    padding: theme.spacing(1, 1, 1, 2),
    backgroundColor: theme.palette.background.paperLight,
    borderBottom: `1px solid ${theme.palette.divider}`,
    color: theme.palette.text.secondary,
    display: "flex",
    flexAlign: "center",
    justifyContent: "space-between",
    alignItems: "center",
  },

  stackCode: {
    padding: theme.spacing(2),
    margin: 0,
    wordWrap: "break-word",
    whiteSpace: "break-spaces",
  },

  copyButton: {
    backgroundColor: "transparent",
    border: 0,
    borderRadius: 999,
    minHeight: theme.spacing(4),
    minWidth: theme.spacing(4),
    height: theme.spacing(4),
    width: theme.spacing(4),

    "& svg": {
      width: 16,
      height: 16,
    },
  },

  version: {
    marginTop: theme.spacing(4),
    fontSize: 12,
    color: theme.palette.text.secondary,
  },
}))
