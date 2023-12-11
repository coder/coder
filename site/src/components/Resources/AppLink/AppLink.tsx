import CircularProgress from "@mui/material/CircularProgress";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline";
import { type FC, useState } from "react";
import { useTheme } from "@emotion/react";
import { getApiKey } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { useProxy } from "contexts/ProxyContext";
import { AgentButton } from "components/Resources/AgentButton";
import { createAppLinkHref } from "utils/apps";
import { generateRandomString } from "utils/random";
import { BaseIcon } from "./BaseIcon";
import { ShareIcon } from "./ShareIcon";

export const DisplayAppNameMap: Record<TypesGen.DisplayApp, string> = {
  port_forwarding_helper: "Ports",
  ssh_helper: "SSH",
  vscode: "VS Code Desktop",
  vscode_insiders: "VS Code Insiders",
  web_terminal: "Terminal",
};

const Language = {
  appTitle: (appName: string, identifier: string): string =>
    `${appName} - ${identifier}`,
};

export interface AppLinkProps {
  workspace: TypesGen.Workspace;
  app: TypesGen.WorkspaceApp;
  agent: TypesGen.WorkspaceAgent;
}

export const AppLink: FC<AppLinkProps> = ({ app, workspace, agent }) => {
  const { proxy } = useProxy();
  const preferredPathBase = proxy.preferredPathAppURL;
  const appsHost = proxy.preferredWildcardHostname;
  const [fetchingSessionToken, setFetchingSessionToken] = useState(false);

  const theme = useTheme();
  const username = workspace.owner_name;

  let appSlug = app.slug;
  let appDisplayName = app.display_name;
  if (!appSlug) {
    appSlug = appDisplayName;
  }
  if (!appDisplayName) {
    appDisplayName = appSlug;
  }

  const href = createAppLinkHref(
    window.location.protocol,
    preferredPathBase,
    appsHost,
    appSlug,
    username,
    workspace,
    agent,
    app,
  );

  let canClick = true;
  let icon = <BaseIcon app={app} />;

  let primaryTooltip = "";
  if (app.health === "initializing") {
    canClick = false;
    icon = <CircularProgress size={12} />;
    primaryTooltip = "Initializing...";
  }
  if (app.health === "unhealthy") {
    canClick = false;
    icon = <ErrorOutlineIcon css={{ color: theme.palette.warning.light }} />;
    primaryTooltip = "Unhealthy";
  }
  if (!appsHost && app.subdomain) {
    canClick = false;
    icon = (
      <ErrorOutlineIcon
        css={{
          color: theme.palette.grey[300],
        }}
      />
    );
    primaryTooltip =
      "Your admin has not configured subdomain application access";
  }
  if (fetchingSessionToken) {
    canClick = false;
  }

  const isPrivateApp = app.sharing_level === "owner";

  const button = (
    <AgentButton
      startIcon={icon}
      endIcon={isPrivateApp ? undefined : <ShareIcon app={app} />}
      disabled={!canClick}
    >
      <span
        css={
          !isPrivateApp && {
            marginRight: 8,
          }
        }
      >
        {appDisplayName}
      </span>
    </AgentButton>
  );

  return (
    <Tooltip title={primaryTooltip}>
      <span>
        <Link
          href={href}
          target="_blank"
          css={{
            pointerEvents: canClick ? undefined : "none",
            textDecoration: "none !important",
          }}
          onClick={
            canClick
              ? async (event) => {
                  event.preventDefault();
                  // This is an external URI like "vscode://", so
                  // it needs to be opened with the browser protocol handler.
                  if (app.external && !app.url.startsWith("http")) {
                    // If the protocol is external the browser does not
                    // redirect the user from the page.

                    // This is a magic undocumented string that is replaced
                    // with a brand-new session token from the backend.
                    // This only exists for external URLs, and should only
                    // be used internally, and is highly subject to break.
                    const magicTokenString = "$SESSION_TOKEN";
                    const hasMagicToken = href.indexOf(magicTokenString);
                    let url = href;
                    if (hasMagicToken !== -1) {
                      setFetchingSessionToken(true);
                      const key = await getApiKey();
                      url = href.replaceAll(magicTokenString, key.key);
                      setFetchingSessionToken(false);
                    }
                    window.location.href = url;
                  } else {
                    window.open(
                      href,
                      Language.appTitle(
                        appDisplayName,
                        generateRandomString(12),
                      ),
                      "width=900,height=600",
                    );
                  }
                }
              : undefined
          }
        >
          {button}
        </Link>
      </span>
    </Tooltip>
  );
};
