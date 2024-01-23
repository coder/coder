import ReplayIcon from "@mui/icons-material/Replay";
import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import { type FC } from "react";
import LoadingButton from "@mui/lab/LoadingButton";
import { visuallyHidden } from "@mui/utils";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { TemplateVersionExternalAuth } from "api/typesGenerated";

export interface ExternalAuthButtonProps {
  auth: TemplateVersionExternalAuth;
  displayRetry: boolean;
  isLoading: boolean;
  onStartPolling: () => void;
}

export const ExternalAuthButton: FC<ExternalAuthButtonProps> = ({
  auth,
  displayRetry,
  isLoading,
  onStartPolling,
}) => {
  return (
    <>
      <div css={{ display: "flex", alignItems: "center", gap: 8 }}>
        <LoadingButton
          fullWidth
          loading={isLoading}
          href={auth.authenticate_url}
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
          {auth.authenticated
            ? `Authenticated with ${auth.display_name}`
            : `Login with ${auth.display_name}`}
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
