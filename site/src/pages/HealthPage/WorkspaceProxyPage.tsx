import { useTheme } from "@emotion/react";
import type { HealthcheckReport } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { GlobeIcon, HashIcon } from "lucide-react";
import type { FC } from "react";
import { useOutletContext } from "react-router";
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
			<title>{pageTitle("Workspace Proxy - Health")}</title>

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
							actions={<HealthMessageDocsLink {...warning} />}
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
								border: `1px solid ${
									region.healthy
										? theme.palette.divider
										: theme.palette.warning.light
								}`,
							}}
							className="rounded-lg text-sm leading-none"
						>
							<header className="p-6 flex items-center justify-between gap-6">
								<div className="flex items-center gap-6">
									<div className="size-9 flex items-center justify-center">
										<img
											src={region.icon_url}
											className="object-fill w-full h-full"
											alt=""
										/>
									</div>
									<div className="leading-relaxed">
										<h4 className="m-0 font-medium">{region.display_name}</h4>
										<span css={{ color: theme.palette.text.secondary }}>
											{region.version}
										</span>
									</div>
								</div>

								<div className="flex flex-wrap gap-3">
									{region.wildcard_hostname && (
										<Tooltip>
											<TooltipTrigger asChild>
												<Pill icon={<GlobeIcon />}>
													{region.wildcard_hostname}
												</Pill>
											</TooltipTrigger>
											<TooltipContent side="bottom">
												Wildcard Hostname
											</TooltipContent>
										</Tooltip>
									)}
									{region.version && (
										<Tooltip>
											<TooltipTrigger asChild>
												<Pill icon={<HashIcon className="size-icon-sm" />}>
													{region.version}
												</Pill>
											</TooltipTrigger>
											<TooltipContent side="bottom">Version</TooltipContent>
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
									color: theme.palette.text.secondary,
								}}
								className="flex items-center justify-between py-2.5 px-6 text-xs leading-relaxed"
							>
								{region.status?.status === "unregistered" ? (
									<span>Has not connected yet</span>
								) : warnings.length === 0 && errors.length === 0 ? (
									<span>OK</span>
								) : (
									<div className="flex flex-col">
										{[...errors, ...warnings].map((msg) => (
											<span key={msg} className="first-letter:uppercase">
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
