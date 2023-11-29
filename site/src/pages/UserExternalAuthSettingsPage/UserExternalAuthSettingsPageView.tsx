import { css } from "@emotion/react";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type {
  ExternalAuthConfig,
  ExternalAuthLinkProvider,
  UserExternalAuthResponse,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EnterpriseBadge } from "components/Badges/Badges";
import { Header } from "components/DeploySettingsLayout/Header";
import { docs } from "utils/docs";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { ExternalAuth } from "pages/CreateWorkspacePage/ExternalAuth";
import { Stack } from "components/Stack/Stack";
import Divider from "@mui/material/Divider";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";

export type UserExternalAuthSettingsPageViewProps = {
  isLoading: boolean;
  getAuthsError?: unknown;
  auths?: UserExternalAuthResponse;
  onUnlinkExternalAuth: (provider: string) => void;
  onValidateExternalAuth: (provider: string) => void;
};

export const UserExternalAuthSettingsPageView = ({
  isLoading,
  getAuthsError,
  auths,
  onUnlinkExternalAuth,
  onValidateExternalAuth,
}: UserExternalAuthSettingsPageViewProps): JSX.Element => {
  if (getAuthsError) {
    // Nothing to show if there is an error
    return <ErrorAlert error={getAuthsError} />;
  }

  if (!auths) {
    // TODO: Do loading?
    return <></>;
  }

  return (
    <>
      <TableContainer>
        <Table
          css={css`
            & td {
              padding-top: 24px;
              padding-bottom: 24px;
            }

            & td:last-child,
            & th:last-child {
              padding-left: 32px;
            }
          `}
        >
          <TableHead>
            <TableRow>
              <TableCell>Application</TableCell>
              <TableCell>Link</TableCell>
              <TableCell></TableCell>
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
                const name = app.id || app.type;
                const link = auths.links.find((l) => l.provider_id === name);
                const authURL = "/external-auth/" + app.id;
                return (
                  <TableRow key={name}>
                    <TableCell>
                      <AvatarData
                        title={app.display_name || app.id}
                        // subtitle={template.description}
                        avatar={
                          app.display_icon !== "" && (
                            <Avatar
                              src={app.display_icon}
                              variant="square"
                              fitImage
                            />
                          )
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <ExternalAuth
                        displayName={name}
                        displayIcon={app.display_icon}
                        authenticated={link ? true : false}
                        authenticateURL={authURL}
                        // TODO: This polling is not implemented
                        externalAuthPollingState="idle"
                        startPollingExternalAuth={() => {}}
                      ></ExternalAuth>
                    </TableCell>
                    <TableCell>
                      {link && (
                        <MoreMenu>
                          <MoreMenuTrigger>
                            <ThreeDotsButton />
                          </MoreMenuTrigger>
                          <MoreMenuContent>
                            <MoreMenuItem
                              onClick={() => {
                                onValidateExternalAuth(app.id);
                              }}
                            >
                              Test Validate&hellip;
                            </MoreMenuItem>
                            <Divider />
                            <MoreMenuItem
                              danger
                              onClick={() => {
                                onUnlinkExternalAuth(app.id);
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
              })}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
};
