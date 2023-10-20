import { Region, WorkspaceProxy } from "api/typesGenerated";
import { AvatarData } from "components/AvatarData/AvatarData";
import { Avatar } from "components/Avatar/Avatar";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import { FC, ReactNode } from "react";
import {
  HealthyBadge,
  NotHealthyBadge,
  NotReachableBadge,
  NotRegisteredBadge,
} from "components/DeploySettingsLayout/Badges";
import { ProxyLatencyReport } from "contexts/useProxyLatency";
import { getLatencyColor } from "utils/latency";
import Box from "@mui/material/Box";

export const ProxyRow: FC<{
  latency?: ProxyLatencyReport;
  proxy: Region;
}> = ({ proxy, latency }) => {
  // If we have a more specific proxy status, use that.
  // All users can see healthy/unhealthy, some can see more.
  let statusBadge = <ProxyStatus proxy={proxy} />;
  let shouldShowMessages = false;
  if ("status" in proxy) {
    const wsproxy = proxy as WorkspaceProxy;
    statusBadge = <DetailedProxyStatus proxy={wsproxy} />;
    shouldShowMessages = Boolean(
      (wsproxy.status?.report?.warnings &&
        wsproxy.status?.report?.warnings.length > 0) ||
        (wsproxy.status?.report?.errors &&
          wsproxy.status?.report?.errors.length > 0),
    );
  }

  return (
    <>
      <TableRow key={proxy.name} data-testid={proxy.name}>
        <TableCell>
          <AvatarData
            title={
              proxy.display_name && proxy.display_name.length > 0
                ? proxy.display_name
                : proxy.name
            }
            avatar={
              proxy.icon_url !== "" && (
                <Avatar
                  size="sm"
                  src={proxy.icon_url}
                  variant="square"
                  fitImage
                />
              )
            }
          />
        </TableCell>

        <TableCell sx={{ fontSize: 14 }}>{proxy.path_app_url}</TableCell>
        <TableCell sx={{ fontSize: 14 }}>{statusBadge}</TableCell>
        <TableCell
          sx={{
            fontSize: 14,
            textAlign: "right",
            color: (theme) =>
              latency
                ? getLatencyColor(theme, latency.latencyMS)
                : theme.palette.text.secondary,
          }}
        >
          {latency ? `${latency.latencyMS.toFixed(0)} ms` : "Not available"}
        </TableCell>
      </TableRow>
      {shouldShowMessages && (
        <TableRow>
          <TableCell
            colSpan={4}
            sx={{ padding: "0px !important", borderBottom: 0 }}
          >
            <ProxyMessagesRow proxy={proxy as WorkspaceProxy} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
};

const ProxyMessagesRow: FC<{
  proxy: WorkspaceProxy;
}> = ({ proxy }) => {
  return (
    <>
      <ProxyMessagesList
        title={
          <Box
            component="span"
            sx={{ color: (theme) => theme.palette.error.light }}
          >
            Errors
          </Box>
        }
        messages={proxy.status?.report?.errors}
      />
      <ProxyMessagesList
        title={
          <Box
            component="span"
            sx={{ color: (theme) => theme.palette.warning.light }}
          >
            Warnings
          </Box>
        }
        messages={proxy.status?.report?.warnings}
      />
    </>
  );
};

const ProxyMessagesList: FC<{
  title: ReactNode;
  messages?: string[];
}> = ({ title, messages }) => {
  if (!messages) {
    return <></>;
  }

  return (
    <Box
      sx={{
        borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
        backgroundColor: (theme) => theme.palette.background.default,
        p: (theme) => theme.spacing(2, 3),
      }}
    >
      <Box
        id="nested-list-subheader"
        sx={{
          mb: 0.5,
          fontSize: 13,
          fontWeight: 600,
        }}
      >
        {title}
      </Box>
      {messages.map((error, index) => (
        <Box
          component="pre"
          key={"message" + index}
          sx={{
            margin: (theme) => theme.spacing(0, 0, 1),
            fontSize: 14,
            whiteSpace: "pre-wrap",
          }}
        >
          {error}
        </Box>
      ))}
    </Box>
  );
};

// DetailedProxyStatus allows a more precise status to be displayed.
const DetailedProxyStatus: FC<{
  proxy: WorkspaceProxy;
}> = ({ proxy }) => {
  if (!proxy.status) {
    // If the status is null/undefined/not provided, just go with the boolean "healthy" value.
    return <ProxyStatus proxy={proxy} />;
  }

  let derpOnly = false;
  if ("derp_only" in proxy) {
    derpOnly = proxy.derp_only;
  }

  switch (proxy.status.status) {
    case "ok":
      return <HealthyBadge derpOnly={derpOnly} />;
    case "unhealthy":
      return <NotHealthyBadge />;
    case "unreachable":
      return <NotReachableBadge />;
    case "unregistered":
      return <NotRegisteredBadge />;
    default:
      return <NotHealthyBadge />;
  }
};

// ProxyStatus will only show "healthy" or "not healthy" status.
const ProxyStatus: FC<{
  proxy: Region;
}> = ({ proxy }) => {
  let icon = <NotHealthyBadge />;
  if (proxy.healthy) {
    icon = <HealthyBadge derpOnly={false} />;
  }

  return icon;
};
