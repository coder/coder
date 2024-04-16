import { useTheme } from "@emotion/react";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type { FC, ReactNode } from "react";
import type { Region, WorkspaceProxy } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import {
  HealthyBadge,
  NotHealthyBadge,
  NotReachableBadge,
  NotRegisteredBadge,
} from "components/Badges/Badges";
import type { ProxyLatencyReport } from "contexts/useProxyLatency";
import { getLatencyColor } from "utils/latency";

interface ProxyRowProps {
  latency?: ProxyLatencyReport;
  proxy: Region;
}

export const ProxyRow: FC<ProxyRowProps> = ({ proxy, latency }) => {
  const theme = useTheme();

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

        <TableCell css={{ fontSize: 14 }}>{proxy.path_app_url}</TableCell>
        <TableCell css={{ fontSize: 14 }}>{statusBadge}</TableCell>
        <TableCell
          css={{
            fontSize: 14,
            textAlign: "right",
            color: latency
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
            css={{ padding: "0 !important", borderBottom: 0 }}
          >
            <ProxyMessagesRow proxy={proxy as WorkspaceProxy} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
};

interface ProxyMessagesRowProps {
  proxy: WorkspaceProxy;
}

const ProxyMessagesRow: FC<ProxyMessagesRowProps> = ({ proxy }) => {
  const theme = useTheme();

  return (
    <>
      <ProxyMessagesList
        title={<span css={{ color: theme.palette.error.light }}>Errors</span>}
        messages={proxy.status?.report?.errors}
      />
      <ProxyMessagesList
        title={
          <span css={{ color: theme.palette.warning.light }}>Warnings</span>
        }
        messages={proxy.status?.report?.warnings}
      />
    </>
  );
};

interface ProxyMessagesListProps {
  title: ReactNode;
  messages?: readonly string[];
}

const ProxyMessagesList: FC<ProxyMessagesListProps> = ({ title, messages }) => {
  const theme = useTheme();

  if (!messages) {
    return <></>;
  }

  return (
    <div
      css={{
        borderBottom: `1px solid ${theme.palette.divider}`,
        backgroundColor: theme.palette.background.default,
        padding: "16px 24px",
      }}
    >
      <div
        id="nested-list-subheader"
        css={{
          marginBottom: 4,
          fontSize: 13,
          fontWeight: 600,
        }}
      >
        {title}
      </div>
      {messages.map((error, index) => (
        <pre
          key={index}
          css={{
            margin: "0 0 8px",
            fontSize: 14,
            whiteSpace: "pre-wrap",
          }}
        >
          {error}
        </pre>
      ))}
    </div>
  );
};

interface DetailedProxyStatusProps {
  proxy: WorkspaceProxy;
}

// DetailedProxyStatus allows a more precise status to be displayed.
const DetailedProxyStatus: FC<DetailedProxyStatusProps> = ({ proxy }) => {
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

interface ProxyStatusProps {
  proxy: Region;
}

// ProxyStatus will only show "healthy" or "not healthy" status.
const ProxyStatus: FC<ProxyStatusProps> = ({ proxy }) => {
  let icon = <NotHealthyBadge />;
  if (proxy.healthy) {
    icon = <HealthyBadge derpOnly={false} />;
  }

  return icon;
};
