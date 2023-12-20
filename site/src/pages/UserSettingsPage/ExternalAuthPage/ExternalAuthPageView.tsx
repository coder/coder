import Divider from "@mui/material/Divider";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { type FC, useState, useCallback, useEffect } from "react";
import { useQuery } from "react-query";
import { externalAuthProvider } from "api/queries/externalAuth";
import type {
  ListUserExternalAuthResponse,
  ExternalAuthLinkProvider,
  ExternalAuthLink,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import { ExternalAuth } from "pages/CreateWorkspacePage/ExternalAuth";
import { ExternalAuthPollingState } from "pages/CreateWorkspacePage/CreateWorkspacePage";

export type ExternalAuthPageViewProps = {
  isLoading: boolean;
  getAuthsError?: unknown;
  unlinked: number;
  auths?: ListUserExternalAuthResponse;
  onUnlinkExternalAuth: (provider: string) => void;
  onValidateExternalAuth: (provider: string) => void;
};

export const ExternalAuthPageView: FC<ExternalAuthPageViewProps> = ({
  isLoading,
  getAuthsError,
  auths,
  unlinked,
  onUnlinkExternalAuth,
  onValidateExternalAuth,
}) => {
  if (getAuthsError) {
    // Nothing to show if there is an error
    return <ErrorAlert error={getAuthsError} />;
  }

  if (isLoading || !auths) {
    return <FullScreenLoader />;
  }

  return (
    <>
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Application</TableCell>
              <TableCell>Link</TableCell>
              <TableCell width="1%"></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {((auths.providers === null || auths.providers?.length === 0) && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div css={{ textAlign: "center" }}>
                    No providers have been configured!
                  </div>
                </TableCell>
              </TableRow>
            )) ||
              auths.providers?.map((app: ExternalAuthLinkProvider) => {
                return (
                  <ExternalAuthRow
                    key={app.id}
                    app={app}
                    unlinked={unlinked}
                    link={auths.links.find((l) => l.provider_id === app.id)}
                    onUnlinkExternalAuth={() => {
                      onUnlinkExternalAuth(app.id);
                    }}
                    onValidateExternalAuth={() => {
                      onValidateExternalAuth(app.id);
                    }}
                  />
                );
              })}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
};

interface ExternalAuthRowProps {
  app: ExternalAuthLinkProvider;
  link?: ExternalAuthLink;
  unlinked: number;
  onUnlinkExternalAuth: () => void;
  onValidateExternalAuth: () => void;
}

const ExternalAuthRow: FC<ExternalAuthRowProps> = ({
  app,
  unlinked,
  link,
  onUnlinkExternalAuth,
  onValidateExternalAuth,
}) => {
  const name = app.id || app.type;
  const authURL = "/external-auth/" + app.id;

  const {
    externalAuth,
    externalAuthPollingState,
    refetch,
    startPollingExternalAuth,
  } = useExternalAuth(app.id, unlinked);

  const authenticated = externalAuth
    ? externalAuth.authenticated
    : link?.authenticated ?? false;

  return (
    <TableRow key={name}>
      <TableCell>
        <AvatarData
          title={app.display_name || app.id}
          avatar={
            app.display_icon && (
              <Avatar src={app.display_icon} variant="square" fitImage />
            )
          }
        />
      </TableCell>
      <TableCell>
        <ExternalAuth
          displayName={name}
          // We could specify the user is linked, but the link is invalid.
          // This could indicate it expired, or was revoked on the other end.
          authenticated={authenticated}
          authenticateURL={authURL}
          displayIcon=""
          message={authenticated ? "Authenticated" : "Click to Login"}
          externalAuthPollingState={externalAuthPollingState}
          startPollingExternalAuth={startPollingExternalAuth}
          fullWidth={false}
        />
      </TableCell>
      <TableCell>
        {(link || externalAuth?.authenticated) && (
          <MoreMenu>
            <MoreMenuTrigger>
              <ThreeDotsButton />
            </MoreMenuTrigger>
            <MoreMenuContent>
              <MoreMenuItem
                onClick={async () => {
                  onValidateExternalAuth();
                  // This is kinda jank. It does a refetch of the thing
                  // it just validated... But we need to refetch to update the
                  // login button. And the 'onValidateExternalAuth' does the
                  // message display.
                  await refetch();
                }}
              >
                Test Validate&hellip;
              </MoreMenuItem>
              <Divider />
              <MoreMenuItem
                danger
                onClick={async () => {
                  onUnlinkExternalAuth();
                  await refetch();
                }}
              >
                Unlink&hellip;
              </MoreMenuItem>
            </MoreMenuContent>
          </MoreMenu>
        )}
      </TableCell>
    </TableRow>
  );
};

// useExternalAuth handles the polling of the auth to update the button.
const useExternalAuth = (providerID: string, unlinked: number) => {
  const [externalAuthPollingState, setExternalAuthPollingState] =
    useState<ExternalAuthPollingState>("idle");

  const startPollingExternalAuth = useCallback(() => {
    setExternalAuthPollingState("polling");
  }, []);

  const { data: externalAuth, refetch } = useQuery({
    ...externalAuthProvider(providerID),
    refetchInterval: externalAuthPollingState === "polling" ? 1000 : false,
  });

  const signedIn = externalAuth?.authenticated;

  useEffect(() => {
    if (unlinked > 0) {
      void refetch();
    }
  }, [refetch, unlinked]);

  useEffect(() => {
    if (signedIn) {
      setExternalAuthPollingState("idle");
      return;
    }

    if (externalAuthPollingState !== "polling") {
      return;
    }

    // Poll for a maximum of one minute
    const quitPolling = setTimeout(
      () => setExternalAuthPollingState("abandoned"),
      60_000,
    );
    return () => {
      clearTimeout(quitPolling);
    };
  }, [externalAuthPollingState, signedIn]);

  return {
    startPollingExternalAuth,
    externalAuth,
    externalAuthPollingState,
    refetch,
  };
};
