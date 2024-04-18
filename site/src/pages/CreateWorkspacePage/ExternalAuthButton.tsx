import ReplayIcon from "@mui/icons-material/Replay";
import LoadingButton from "@mui/lab/LoadingButton";
import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import { visuallyHidden } from "@mui/utils";
import type { FC } from "react";
import type { TemplateVersionExternalAuth } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Pill } from "components/Pill/Pill";

export interface ExternalAuthButtonProps {
  auth: TemplateVersionExternalAuth;
  displayRetry: boolean;
  isLoading: boolean;
  onStartPolling: () => void;
  error?: unknown;
}

export const ExternalAuthButton: FC<ExternalAuthButtonProps> = ({
  auth,
  displayRetry,
  isLoading,
  onStartPolling,
  error,
}) => {
  return (
    <>
      <div css={{ display: "flex", alignItems: "center", gap: 8 }}>
        <LoadingButton
          fullWidth
          loading={isLoading}
          variant="contained"
          size="xlarge"
          startIcon={
            auth.display_icon && (
              <ExternalImage
                src={auth.display_icon}
                alt={`${auth.display_name} Icon`}
                css={{ width: 16, height: 16 }}
              />
            )
          }
          disabled={auth.authenticated}
          onClick={() => {
            window.open(
              auth.authenticate_url,
              "_blank",
              "width=900,height=600",
            );
            onStartPolling();
          }}
        >
          {auth.authenticated ? (
            `Authenticated with ${auth.display_name}`
          ) : (
            <>
              Login with {auth.display_name}
              {!auth.optional && (
                <Pill type={error ? "error" : "info"} css={{ marginLeft: 12 }}>
                  Required
                </Pill>
              )}
            </>
          )}
        </LoadingButton>

        {displayRetry && (
          <Tooltip title="Retry">
            <Button
              variant="contained"
              size="xlarge"
              onClick={onStartPolling}
              css={{ minWidth: "auto", aspectRatio: "1" }}
            >
              <ReplayIcon css={{ width: 20, height: 20 }} />
              <span aria-hidden css={{ ...visuallyHidden }}>
                Refresh external auth
              </span>
            </Button>
          </Tooltip>
        )}
      </div>
    </>
  );
};
