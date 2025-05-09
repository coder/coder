import { useTheme } from "@emotion/react";
import PublicOutlined from "@mui/icons-material/PublicOutlined";
import TagOutlined from "@mui/icons-material/TagOutlined";
import Tooltip from "@mui/material/Tooltip";
import type { HealthcheckReport } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useOutletContext } from "react-router-dom";
import { createDayString } from "utils/createDayString";
import { pageTitle } from "utils/page";
import {
	BooleanPill,
	Header,
	HeaderTitle,
	HealthMessageDocsLink,
	HealthyDot,
	Main,
	Pill,
} from "./Content";
import { DismissWarningButton } from "./DismissWarningButton";

const WorkspaceProxyPage: FC = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const { workspace_proxy } = healthStatus;
	const { regions } = workspace_proxy.workspace_proxies;
	const theme = useTheme();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Workspace Proxy - Health")}</title>
			</Helmet>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={workspace_proxy.severity} />
					Workspace Proxy
				</HeaderTitle>
				<DismissWarningButton healthcheck="WorkspaceProxy" />
			</Header>

			<Main>
				{workspace_proxy.error && (
					<Alert severity="error">{workspace_proxy.error}</Alert>
				)}
				{workspace_proxy.warnings.map((warning) => {
					return (
						<Alert
							actions={HealthMessageDocsLink(warning)}
							key={warning.code}
							severity="warning"
						>
							{warning.message}
						</Alert>
					);
				})}

				{regions.map((region) => {
					const errors = region.status?.report?.errors ?? [];
					const warnings = region.status?.report?.warnings ?? [];

					return (
						<div
							key={region.id}
							css={{
								borderRadius: 8,
								border: `1px solid ${
									region.healthy
										? theme.palette.divider
										: theme.palette.warning.light
								}`,
								fontSize: 14,
							}}
						>
							<header
								css={{
									padding: 24,
									display: "flex",
									alignItems: "center",
									justifyContent: "space-between",
									gap: 24,
								}}
							>
								<div css={{ display: "flex", alignItems: "center", gap: 24 }}>
									<div
										css={{
											width: 36,
											height: 36,
											display: "flex",
											alignItems: "center",
											justifyContent: "center",
										}}
									>
										<img
											src={region.icon_url}
											css={{ objectFit: "fill", width: "100%", height: "100%" }}
											alt=""
										/>
									</div>
									<div css={{ lineHeight: "160%" }}>
										<h4 css={{ fontWeight: 500, margin: 0 }}>
											{region.display_name}
										</h4>
										<span css={{ color: theme.palette.text.secondary }}>
											{region.version}
										</span>
									</div>
								</div>

								<div css={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
									{region.wildcard_hostname && (
										<Tooltip title="Wildcard Hostname">
											<Pill icon={<PublicOutlined />}>
												{region.wildcard_hostname}
											</Pill>
										</Tooltip>
									)}
									{region.version && (
										<Tooltip title="Version">
											<Pill icon={<TagOutlined />}>{region.version}</Pill>
										</Tooltip>
									)}
									{region.derp_enabled && (
										<BooleanPill value={region.derp_enabled}>
											DERP Enabled
										</BooleanPill>
									)}
									{region.derp_only && (
										<BooleanPill value={region.derp_only}>
											DERP Only
										</BooleanPill>
									)}
									{region.deleted && (
										<BooleanPill value={region.deleted}>Deleted</BooleanPill>
									)}
								</div>
							</header>

							<div
								css={{
									borderTop: `1px solid ${theme.palette.divider}`,
									display: "flex",
									alignItems: "center",
									justifyContent: "space-between",
									padding: "8px 24px",
									fontSize: 12,
									color: theme.palette.text.secondary,
								}}
							>
								{region.status?.status === "unregistered" ? (
									<span>Has not connected yet</span>
								) : warnings.length === 0 && errors.length === 0 ? (
									<span>OK</span>
								) : (
									<div css={{ display: "flex", flexDirection: "column" }}>
										{[...errors, ...warnings].map((msg) => (
											<span
												key={msg}
												css={{
													":first-letter": { textTransform: "uppercase" },
												}}
											>
												{msg}
											</span>
										))}
									</div>
								)}
								<span data-chromatic="ignore">
									{createDayString(region.updated_at)}
								</span>
							</div>
						</div>
					);
				})}
			</Main>
		</>
	);
};

export default WorkspaceProxyPage;
