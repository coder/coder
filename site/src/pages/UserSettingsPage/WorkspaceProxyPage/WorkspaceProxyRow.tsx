import { useTheme } from "@emotion/react";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type { Region, WorkspaceProxy } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import {
	HealthyBadge,
	NotHealthyBadge,
	NotReachableBadge,
	NotRegisteredBadge,
} from "components/Badges/Badges";
import type { ProxyLatencyReport } from "contexts/useProxyLatency";
import type { FC, ReactNode } from "react";
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
	const extraWarnings: string[] = [];
	if (latency?.nextHopProtocol) {
		switch (latency.nextHopProtocol) {
			case "http/0.9":
			case "http/1.0":
			case "http/1.1":
				extraWarnings.push(
					// biome-ignore lint/style/useTemplate: easier to read short lines
					`Requests to the proxy from current browser are using "${latency.nextHopProtocol}". ` +
						"The proxy server might not support HTTP/2. " +
						"For usability reasons, HTTP/2 or above is recommended. " +
						"Pages may fail to load if the web browser's concurrent " +
						"connection limit per host is reached.",
				);
		}
	}

	if ("status" in proxy) {
		const wsproxy = proxy as WorkspaceProxy;
		statusBadge = <DetailedProxyStatus proxy={wsproxy} />;
		shouldShowMessages = Boolean(
			(wsproxy.status?.report?.warnings &&
				wsproxy.status?.report?.warnings.length > 0) ||
				extraWarnings.length > 0 ||
				(wsproxy.status?.report?.errors &&
					wsproxy.status?.report?.errors.length > 0),
		);
	}

	return (
		<>
			<TableRow key={proxy.name} data-testid={proxy.name}>
				<TableCell className="summary">
					<AvatarData
						src={proxy.icon_url}
						title={proxy.display_name || proxy.name}
						subtitle={proxy.path_app_url}
						avatar={
							<Avatar
								variant="icon"
								src={proxy.icon_url}
								fallback={proxy.display_name || proxy.name}
							/>
						}
					/>
				</TableCell>

				<TableCell className="status">
					<div className="flex items-center justify-end">{statusBadge}</div>
				</TableCell>
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
						<ProxyMessagesRow
							proxy={proxy as WorkspaceProxy}
							extraWarnings={extraWarnings}
						/>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};

interface ProxyMessagesRowProps {
	proxy: WorkspaceProxy;
	extraWarnings: string[];
}

const ProxyMessagesRow: FC<ProxyMessagesRowProps> = ({
	proxy,
	extraWarnings,
}) => {
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
				messages={[...(proxy.status?.report?.warnings ?? []), ...extraWarnings]}
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
				padding: "16px 64px",
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
