import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import RefreshOutlined from "@mui/icons-material/RefreshOutlined";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { css } from "@emotion/css";
import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import type { BuildInfoResponse } from "api/typesGenerated";
import { CopyButton } from "components/CopyButton/CopyButton";
import { CoderIcon } from "components/Icons/CoderIcon";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import { Stack } from "components/Stack/Stack";
import { Margins } from "components/Margins/Margins";

const fetchDynamicallyImportedModuleError =
  "Failed to fetch dynamically imported module";

export type RuntimeErrorStateProps = { error: Error };

export const RuntimeErrorState: FC<RuntimeErrorStateProps> = ({ error }) => {
  const theme = useTheme();
  const [checkingError, setCheckingError] = useState(true);
  const [staticBuildInfo, setStaticBuildInfo] = useState<BuildInfoResponse>();
  const coderVersion = staticBuildInfo?.version;

  // We use an effect to show a loading state if the page is trying to reload
  useEffect(() => {
    const isImportError = error.message.includes(
      fetchDynamicallyImportedModuleError,
    );
    const isRetried = window.location.search.includes("retries=1");

    if (isImportError && !isRetried) {
      const url = new URL(location.href);
      // Add a retry to avoid loops
      url.searchParams.set("retries", "1");
      location.assign(url.search);
      return;
    }

    setCheckingError(false);
  }, [error.message]);

  useEffect(() => {
    if (!checkingError) {
      setStaticBuildInfo(getStaticBuildInfo());
    }
  }, [checkingError]);

  return (
    <>
      <Helmet>
        <title>Something went wrong...</title>
      </Helmet>
      {!checkingError ? (
        <Margins css={styles.root}>
          <div css={{ width: "100%" }}>
            <CoderIcon css={styles.logo} />
            <h1 css={styles.title}>Something went wrong...</h1>
            <p css={styles.text}>
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
                  window.location.reload();
                }}
              >
                Reload page
              </Button>
              <Button component="a" href="/">
                Go to dashboard
              </Button>
            </Stack>
            {error.stack && (
              <div css={styles.stack}>
                <div css={styles.stackHeader}>
                  Stacktrace
                  <CopyButton
                    buttonClassName={css`
                      background-color: transparent;
                      border: 0;
                      border-radius: 999px;
                      min-height: ${theme.spacing(4)};
                      min-width: ${theme.spacing(4)};
                      height: ${theme.spacing(4)};
                      width: ${theme.spacing(4)};

                      & svg {
                        width: 16px;
                        height: 16px;
                      }
                    `}
                    text={error.stack}
                    tooltipTitle="Copy stacktrace"
                  />
                </div>
                <pre css={styles.stackCode}>{error.stack}</pre>
              </div>
            )}
            {coderVersion && (
              <div css={styles.version}>Version: {coderVersion}</div>
            )}
          </div>
        </Margins>
      ) : (
        <FullScreenLoader />
      )}
    </>
  );
};

// During the build process, we inject the build info into the HTML
const getStaticBuildInfo = () => {
  const buildInfoJson = document
    .querySelector("meta[property=build-info]")
    ?.getAttribute("content");

  if (buildInfoJson) {
    try {
      return JSON.parse(buildInfoJson) as BuildInfoResponse;
    } catch {
      return undefined;
    }
  }
};

const styles = {
  root: (theme) => ({
    paddingTop: theme.spacing(4),
    paddingBottom: theme.spacing(4),
    textAlign: "center",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    minHeight: "100%",
    maxWidth: theme.spacing(75),
  }),

  logo: (theme) => ({
    fontSize: theme.spacing(8),
  }),

  title: (theme) => ({
    fontSize: theme.spacing(4),
    fontWeight: 400,
  }),

  text: (theme) => ({
    fontSize: 16,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    marginBottom: theme.spacing(4),
  }),

  stack: (theme) => ({
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 4,
    marginTop: theme.spacing(8),
    display: "block",
    textAlign: "left",
  }),

  stackHeader: (theme) => ({
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
  }),

  stackCode: (theme) => ({
    padding: theme.spacing(2),
    margin: 0,
    wordWrap: "break-word",
    whiteSpace: "break-spaces",
  }),

  version: (theme) => ({
    marginTop: theme.spacing(4),
    fontSize: 12,
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
