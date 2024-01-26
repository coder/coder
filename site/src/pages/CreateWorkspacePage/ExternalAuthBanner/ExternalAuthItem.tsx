import { Interpolation, Theme } from "@emotion/react";
import DoneAllOutlined from "@mui/icons-material/DoneAllOutlined";
import LoadingButton from "@mui/lab/LoadingButton";
import { TemplateVersionExternalAuth } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { FC, useEffect, useState } from "react";
// eslint-disable-next-line no-restricted-imports -- used to allow extension with "component"
import Box, { BoxProps } from "@mui/material/Box";

type Status = "idle" | "connecting";

type ExternalAuthItemProps = {
  provider: TemplateVersionExternalAuth;
  isPolling: boolean;
  defaultStatus?: Status;
  onStartPolling: () => void;
} & BoxProps;

export const ExternalAuthItem: FC<ExternalAuthItemProps> = ({
  provider,
  isPolling,
  defaultStatus = "idle",
  onStartPolling,
  ...boxProps
}) => {
  const [status, setStatus] = useState(defaultStatus);

  useEffect(() => {
    if (!isPolling) {
      setStatus("idle");
    }
  }, [isPolling]);

  return (
    <Box key={provider.id} css={styles.providerItem} {...boxProps}>
      <span css={styles.providerHeader}>
        <ExternalImage src={provider.display_icon} css={styles.providerIcon} />
        <strong css={styles.providerName}>{provider.display_name}</strong>
      </span>
      {provider.authenticated ? (
        <span css={styles.providerConnectedLabel}>
          Connected
          <DoneAllOutlined css={styles.providerConnectedLabelIcon} />
        </span>
      ) : (
        <LoadingButton
          loading={status === "connecting"}
          size="small"
          css={styles.connectButton}
          variant="contained"
          color="primary"
          onClick={() => {
            setStatus("connecting");
            window.open(
              provider.authenticate_url,
              "_blank",
              "width=900,height=600",
            );
            onStartPolling();
          }}
        >
          Connect&hellip;
        </LoadingButton>
      )}
    </Box>
  );
};

const styles = {
  providerItem: (theme) => ({
    display: "flex",
    alignItems: "center",
    padding: "8px 8px 8px 20px",
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 6,
    justifyContent: "space-between",
    gap: 24,
    fontSize: 14,
  }),

  providerHeader: {
    display: "flex",
    alignItems: "center",
    gap: 12,
    flex: 1,
    overflow: "hidden",
  },

  providerName: {
    fontWeight: 500,
    display: "block",
    whiteSpace: "nowrap",
    maxWidth: "100%",
    textOverflow: "ellipsis",
    overflow: "hidden",
  },

  providerIcon: {
    width: 16,
    height: 16,
  },

  connectButton: {
    flexShrink: 0,
    borderRadius: 4,
  },

  providerConnectedLabel: (theme) => ({
    fontSize: 13,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.disabled,
    gap: 8,
    // Have the same height of the button
    height: 32,
    // Better visual alignment
    padding: "0 8px",
  }),

  providerConnectedLabelIcon: (theme) => ({
    color: theme.experimental.roles.success.fill,
    fontSize: 16,
  }),
} as Record<string, Interpolation<Theme>>;
