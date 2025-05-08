import { CancelOutlined as CancelOutlined, CheckCircleOutlined as CheckCircleOutlined, LinkOutlined as LinkOutlined } from "lucide-react";
import { useTheme } from "@emotion/react";
import LinearProgress from "@mui/material/LinearProgress";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import { entitlements } from "api/queries/entitlements";
import {
	insightsTemplate,
	insightsUserActivity,
	insightsUserLatency,
} from "api/queries/insights";
import type {
	Entitlements,
	Template,
	TemplateAppUsage,
	TemplateInsightsResponse,
	TemplateParameterUsage,
	TemplateParameterValue,
	UserActivityInsightsResponse,
	UserLatencyInsightsResponse,
} from "api/typesGenerated";
import chroma from "chroma-js";
import {
	ActiveUserChart,
	ActiveUsersTitle,
} from "components/ActiveUserChart/ActiveUserChart";
import { Avatar } from "components/Avatar/Avatar";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import {
	addHours,
	addWeeks,
	format,
	startOfDay,
	startOfHour,
	subDays,
} from "date-fns";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import {
	type FC,
	type HTMLAttributes,
	type PropsWithChildren,
	type ReactNode,
	useId,
} from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import { getLatencyColor } from "utils/latency";
import { getTemplatePageTitle } from "../utils";
import { DateRange as DailyPicker, type DateRangeValue } from "./DateRange";
import { type InsightsInterval, IntervalMenu } from "./IntervalMenu";
import { WeekPicker, numberOfWeeksOptions } from "./WeekPicker";
import { lastWeeks } from "./utils";

const DEFAULT_NUMBER_OF_WEEKS = numberOfWeeksOptions[0];

export default function TemplateInsightsPage() {
	const { template } = useTemplateLayoutContext();
	const [searchParams, setSearchParams] = useSearchParams();

	const defaultInterval = getDefaultInterval(template);
	const interval =
		(searchParams.get("interval") as InsightsInterval) || defaultInterval;

	const dateRange = getDateRange(searchParams, interval);
	const setDateRange = (newDateRange: DateRangeValue) => {
		searchParams.set("startDate", newDateRange.startDate.toISOString());
		searchParams.set("endDate", newDateRange.endDate.toISOString());
		setSearchParams(searchParams);
	};

	// date ranges can have different offsets because of daylight savings so to
	// avoid that we are going to use a common offset
	const baseOffset = dateRange.endDate.getTimezoneOffset();
	const commonFilters = {
		template_ids: template.id,
		start_time: toISOLocal(dateRange.startDate, baseOffset),
		end_time: toISOLocal(dateRange.endDate, baseOffset),
	};

	const insightsFilter = { ...commonFilters, interval };
	const { data: templateInsights } = useQuery(insightsTemplate(insightsFilter));
	const { data: userLatency } = useQuery(insightsUserLatency(commonFilters));
	const { data: userActivity } = useQuery(insightsUserActivity(commonFilters));

	const { metadata } = useEmbeddedMetadata();
	const { data: entitlementsQuery } = useQuery(
		entitlements(metadata.entitlements),
	);

	return (
		<>
			<Helmet>
				<title>{getTemplatePageTitle("Insights", template)}</title>
			</Helmet>
			<TemplateInsightsPageView
				controls={
					<>
						<IntervalMenu
							value={interval}
							onChange={(interval) => {
								// When going from daily to week we need to set a safe week range
								if (interval === "week") {
									setDateRange(lastWeeks(DEFAULT_NUMBER_OF_WEEKS));
								}
								searchParams.set("interval", interval);
								setSearchParams(searchParams);
							}}
						/>
						{interval === "day" ? (
							<DailyPicker value={dateRange} onChange={setDateRange} />
						) : (
							<WeekPicker value={dateRange} onChange={setDateRange} />
						)}
					</>
				}
				templateInsights={templateInsights}
				userLatency={userLatency}
				userActivity={userActivity}
				interval={interval}
				entitlements={entitlementsQuery}
			/>
		</>
	);
}

const getDefaultInterval = (template: Template) => {
	const now = new Date();
	const templateCreateDate = new Date(template.created_at);
	const hasFiveWeeksOrMore = addWeeks(templateCreateDate, 5) < now;
	return hasFiveWeeksOrMore ? "week" : "day";
};

const getDateRange = (
	searchParams: URLSearchParams,
	interval: InsightsInterval,
) => {
	const startDate = searchParams.get("startDate");
	const endDate = searchParams.get("endDate");

	if (startDate && endDate) {
		return {
			startDate: new Date(startDate),
			endDate: new Date(endDate),
		};
	}

	if (interval === "day") {
		// Only instantiate new Date once so that we don't get the wrong interval if
		// start is 23:59:59.999 and the clock shifts to 00:00:00 before the second
		// instantiation.
		const today = new Date();
		return {
			startDate: startOfDay(subDays(today, 6)),
			// Add one hour to endDate to include real-time data for today.
			endDate: addHours(startOfHour(today), 1),
		};
	}

	return lastWeeks(DEFAULT_NUMBER_OF_WEEKS);
};

interface TemplateInsightsPageViewProps {
	templateInsights: TemplateInsightsResponse | undefined;
	userLatency: UserLatencyInsightsResponse | undefined;
	userActivity: UserActivityInsightsResponse | undefined;
	entitlements: Entitlements | undefined;
	controls: ReactNode;
	interval: InsightsInterval;
}

export const TemplateInsightsPageView: FC<TemplateInsightsPageViewProps> = ({
	templateInsights,
	userLatency,
	userActivity,
	entitlements,
	controls,
	interval,
}) => {
	return (
		<>
			<div
				css={{
					marginBottom: 32,
					display: "flex",
					alignItems: "center",
					gap: 8,
				}}
			>
				{controls}
			</div>
			<div
				css={{
					display: "grid",
					gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
					gridTemplateRows: "440px 440px auto",
					gap: 24,
				}}
			>
				<ActiveUsersPanel
					css={{ gridColumn: "span 2" }}
					interval={interval}
					userLimit={
						entitlements?.features.user_limit.enabled
							? entitlements?.features.user_limit.limit
							: undefined
					}
					data={templateInsights?.interval_reports}
				/>
				<UsersLatencyPanel data={userLatency} />
				<TemplateUsagePanel
					css={{ gridColumn: "span 2" }}
					data={templateInsights?.report?.apps_usage}
				/>
				<UsersActivityPanel data={userActivity} />
				<TemplateParametersUsagePanel
					css={{ gridColumn: "span 3" }}
					data={templateInsights?.report?.parameters_usage}
				/>
			</div>
		</>
	);
};

interface ActiveUsersPanelProps extends PanelProps {
	data: TemplateInsightsResponse["interval_reports"] | undefined;
	interval: InsightsInterval;
	userLimit: number | undefined;
}

const ActiveUsersPanel: FC<ActiveUsersPanelProps> = ({
	data,
	interval,
	userLimit,
	...panelProps
}) => {
	return (
		<Panel {...panelProps}>
			<PanelHeader>
				<PanelTitle>
					<ActiveUsersTitle interval={interval} />
				</PanelTitle>
			</PanelHeader>
			<PanelContent>
				{!data && <Loader css={{ height: "100%" }} />}
				{data && data.length === 0 && <NoDataAvailable />}
				{data && data.length > 0 && (
					<ActiveUserChart
						interval={interval}
						data={data.map((d) => ({
							amount: d.active_users,
							date: d.start_time,
						}))}
					/>
				)}
			</PanelContent>
		</Panel>
	);
};

interface UsersLatencyPanelProps extends PanelProps {
	data: UserLatencyInsightsResponse | undefined;
}

const UsersLatencyPanel: FC<UsersLatencyPanelProps> = ({
	data,
	...panelProps
}) => {
	const theme = useTheme();
	const users = data?.report.users;

	return (
		<Panel {...panelProps} css={{ overflowY: "auto" }}>
			<PanelHeader>
				<PanelTitle css={{ display: "flex", alignItems: "center", gap: 8 }}>
					Latency by user
					<HelpTooltip>
						<HelpTooltipTrigger size="small" />
						<HelpTooltipContent>
							<HelpTooltipTitle>How is latency calculated?</HelpTooltipTitle>
							<HelpTooltipText>
								The median round trip time of user connections to workspaces.
							</HelpTooltipText>
						</HelpTooltipContent>
					</HelpTooltip>
				</PanelTitle>
			</PanelHeader>

			<PanelContent>
				{!data && <Loader css={{ height: "100%" }} />}
				{users && users.length === 0 && <NoDataAvailable />}
				{users &&
					[...users]
						.sort((a, b) => b.latency_ms.p50 - a.latency_ms.p50)
						.map((row) => (
							<div
								key={row.user_id}
								css={{
									display: "flex",
									justifyContent: "space-between",
									alignItems: "center",
									fontSize: 14,
									paddingTop: 8,
									paddingBottom: 8,
								}}
							>
								<div css={{ display: "flex", alignItems: "center", gap: 12 }}>
									<Avatar fallback={row.username} src={row.avatar_url} />
									<div css={{ fontWeight: 500 }}>{row.username}</div>
								</div>
								<div
									css={{
										color: getLatencyColor(theme, row.latency_ms.p50),
										fontWeight: 500,
										fontSize: 13,
										textAlign: "right",
									}}
								>
									{row.latency_ms.p50.toFixed(0)}ms
								</div>
							</div>
						))}
			</PanelContent>
		</Panel>
	);
};

interface UsersActivityPanelProps extends PanelProps {
	data: UserActivityInsightsResponse | undefined;
}

const UsersActivityPanel: FC<UsersActivityPanelProps> = ({
	data,
	...panelProps
}) => {
	const theme = useTheme();

	const users = data?.report.users;

	return (
		<Panel {...panelProps} css={{ overflowY: "auto" }}>
			<PanelHeader>
				<PanelTitle css={{ display: "flex", alignItems: "center", gap: 8 }}>
					Activity by user
					<HelpTooltip>
						<HelpTooltipTrigger size="small" />
						<HelpTooltipContent>
							<HelpTooltipTitle>How is activity calculated?</HelpTooltipTitle>
							<HelpTooltipText>
								When a connection is initiated to a user&apos;s workspace they
								are considered an active user. e.g. apps, web terminal, SSH
							</HelpTooltipText>
						</HelpTooltipContent>
					</HelpTooltip>
				</PanelTitle>
			</PanelHeader>
			<PanelContent>
				{!data && <Loader css={{ height: "100%" }} />}
				{users && users.length === 0 && <NoDataAvailable />}
				{users &&
					[...users]
						.sort((a, b) => b.seconds - a.seconds)
						.map((row) => (
							<div
								key={row.user_id}
								css={{
									display: "flex",
									justifyContent: "space-between",
									alignItems: "center",
									fontSize: 14,
									paddingTop: 8,
									paddingBottom: 8,
								}}
							>
								<div css={{ display: "flex", alignItems: "center", gap: 12 }}>
									<Avatar fallback={row.username} src={row.avatar_url} />
									<div css={{ fontWeight: 500 }}>{row.username}</div>
								</div>
								<div
									css={{
										color: theme.palette.text.secondary,
										fontSize: 13,
										textAlign: "right",
									}}
								>
									{formatTime(row.seconds)}
								</div>
							</div>
						))}
			</PanelContent>
		</Panel>
	);
};

interface TemplateUsagePanelProps extends PanelProps {
	data: readonly TemplateAppUsage[] | undefined;
}

const TemplateUsagePanel: FC<TemplateUsagePanelProps> = ({
	data,
	...panelProps
}) => {
	const theme = useTheme();
	const validUsage = data
		?.filter((u) => u.seconds > 0)
		.sort((a, b) => b.seconds - a.seconds);
	const totalInSeconds =
		validUsage?.reduce((total, usage) => total + usage.seconds, 0) ?? 1;
	const usageColors = chroma
		.scale([theme.roles.success.fill.solid, theme.roles.warning.fill.solid])
		.mode("lch")
		.colors(validUsage?.length ?? 0);
	// The API returns a row for each app, even if the user didn't use it.
	const hasDataAvailable = validUsage && validUsage.length > 0;

	return (
		<Panel {...panelProps} css={{ overflowY: "auto" }}>
			<PanelHeader>
				<PanelTitle>App & IDE Usage</PanelTitle>
			</PanelHeader>
			<PanelContent>
				{!data && <Loader css={{ height: "100%" }} />}
				{data && !hasDataAvailable && <NoDataAvailable />}
				{data && hasDataAvailable && (
					<div
						css={{
							display: "flex",
							flexDirection: "column",
							gap: 24,
						}}
					>
						{validUsage.map((usage, i) => {
							const percentage = (usage.seconds / totalInSeconds) * 100;
							return (
								<div
									key={usage.slug}
									css={{ display: "flex", gap: 24, alignItems: "center" }}
								>
									<div css={{ display: "flex", alignItems: "center", gap: 8 }}>
										<div
											css={{
												width: 20,
												height: 20,
												display: "flex",
												alignItems: "center",
												justifyContent: "center",
											}}
										>
											<img
												src={usage.icon}
												alt=""
												style={{
													objectFit: "contain",
													width: "100%",
													height: "100%",
												}}
											/>
										</div>
										<div css={{ fontSize: 13, fontWeight: 500, width: 200 }}>
											{usage.display_name}
										</div>
									</div>
									<Tooltip
										title={`${Math.floor(percentage)}%`}
										placement="top"
										arrow
									>
										<LinearProgress
											value={percentage}
											variant="determinate"
											css={{
												width: "100%",
												height: 8,
												backgroundColor: theme.palette.divider,
												"& .MuiLinearProgress-bar": {
													backgroundColor: usageColors[i],
													borderRadius: 999,
												},
											}}
										/>
									</Tooltip>
									<Stack
										spacing={0}
										css={{
											fontSize: 13,
											color: theme.palette.text.secondary,
											width: 120,
											flexShrink: 0,
											lineHeight: "1.5",
										}}
									>
										{formatTime(usage.seconds)}
										{usage.times_used > 0 && (
											<span
												css={{
													fontSize: 12,
													color: theme.palette.text.disabled,
												}}
											>
												Opened {usage.times_used.toLocaleString()}{" "}
												{usage.times_used === 1 ? "time" : "times"}
											</span>
										)}
									</Stack>
								</div>
							);
						})}
					</div>
				)}
			</PanelContent>
		</Panel>
	);
};

interface TemplateParametersUsagePanelProps extends PanelProps {
	data: readonly TemplateParameterUsage[] | undefined;
}

const TemplateParametersUsagePanel: FC<TemplateParametersUsagePanelProps> = ({
	data,
	...panelProps
}) => {
	const theme = useTheme();

	return (
		<Panel {...panelProps}>
			<PanelHeader>
				<PanelTitle>Parameters usage</PanelTitle>
			</PanelHeader>
			<PanelContent>
				{!data && <Loader css={{ height: 200 }} />}
				{data && data.length === 0 && <NoDataAvailable css={{ height: 200 }} />}
				{data &&
					data.length > 0 &&
					data.map((parameter, parameterIndex) => {
						const label =
							parameter.display_name !== ""
								? parameter.display_name
								: parameter.name;
						return (
							<div
								key={parameter.name}
								css={{
									display: "flex",
									alignItems: "start",
									padding: 24,
									marginLeft: -24,
									marginRight: -24,
									borderTop: `1px solid ${theme.palette.divider}`,
									width: "calc(100% + 48px)",
									"&:first-child": {
										borderTop: 0,
									},
									gap: 24,
								}}
							>
								<div css={{ flex: 1 }}>
									<div css={{ fontWeight: 500 }}>{label}</div>
									<p
										css={{
											fontSize: 14,
											color: theme.palette.text.secondary,
											maxWidth: 400,
											margin: 0,
										}}
									>
										{parameter.description}
									</p>
								</div>
								<div css={{ flex: 1, fontSize: 14, flexGrow: 2 }}>
									<ParameterUsageRow
										css={{
											color: theme.palette.text.secondary,
											fontWeight: 500,
											fontSize: 13,
											cursor: "default",
										}}
									>
										<div>Value</div>
										<Tooltip
											title="The number of workspaces using this value"
											placement="top"
										>
											<div>Count</div>
										</Tooltip>
									</ParameterUsageRow>
									{[...parameter.values]
										.sort((a, b) => b.count - a.count)
										.filter((usage) => filterOrphanValues(usage, parameter))
										.map((usage, usageIndex) => (
											<ParameterUsageRow
												key={`${parameterIndex}-${usageIndex}`}
											>
												<ParameterUsageLabel
													usage={usage}
													parameter={parameter}
												/>
												<div css={{ textAlign: "right" }}>{usage.count}</div>
											</ParameterUsageRow>
										))}
								</div>
							</div>
						);
					})}
			</PanelContent>
		</Panel>
	);
};

const filterOrphanValues = (
	usage: TemplateParameterValue,
	parameter: TemplateParameterUsage,
) => {
	if (parameter.options) {
		return parameter.options.some((o) => o.value === usage.value);
	}
	return true;
};

const ParameterUsageRow: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	return (
		<div
			css={{
				display: "flex",
				alignItems: "baseline",
				justifyContent: "space-between",
				padding: "4px 0",
			}}
			{...attrs}
		>
			{children}
		</div>
	);
};

interface ParameterUsageLabelProps {
	usage: TemplateParameterValue;
	parameter: TemplateParameterUsage;
}

const ParameterUsageLabel: FC<ParameterUsageLabelProps> = ({
	usage,
	parameter,
}) => {
	const ariaId = useId();
	const theme = useTheme();

	if (parameter.options) {
		const option = parameter.options.find((o) => o.value === usage.value)!;
		const icon = option.icon;
		const label = option.name;

		return (
			<div
				css={{
					display: "flex",
					alignItems: "center",
					gap: 16,
				}}
			>
				{icon && (
					<div css={{ width: 16, height: 16, lineHeight: 1 }}>
						<img
							alt=""
							src={icon}
							css={{
								objectFit: "contain",
								width: "100%",
								height: "100%",
							}}
							aria-labelledby={ariaId}
						/>
					</div>
				)}
				<span id={ariaId}>{label}</span>
			</div>
		);
	}

	if (usage.value.startsWith("http")) {
		return (
			<Link
				href={usage.value}
				target="_blank"
				rel="noreferrer"
				css={{
					display: "flex",
					alignItems: "center",
					gap: 1,
					color: theme.palette.text.primary,
				}}
			>
				<TextValue>{usage.value}</TextValue>
				<LinkOutlined
					css={{
						width: 14,
						height: 14,
						color: theme.palette.primary.light,
					}}
				/>
			</Link>
		);
	}

	if (parameter.type === "list(string)") {
		const values = JSON.parse(usage.value) as string[];
		return (
			<div css={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
				{values.map((v, i) => (
					<div
						key={i}
						css={{
							padding: "2px 12px",
							borderRadius: 999,
							background: theme.palette.divider,
							whiteSpace: "nowrap",
						}}
					>
						{v}
					</div>
				))}
			</div>
		);
	}

	if (parameter.type === "bool") {
		return (
			<div
				css={{
					display: "flex",
					alignItems: "center",
					gap: 8,
				}}
			>
				{usage.value === "false" ? (
					<>
						<CancelOutlined
							css={{
								width: 16,
								height: 16,
								color: theme.palette.error.light,
							}}
						/>
						False
					</>
				) : (
					<>
						<CheckCircleOutlined
							css={{
								width: 16,
								height: 16,
								color: theme.palette.success.light,
							}}
						/>
						True
					</>
				)}
			</div>
		);
	}

	return <TextValue>{usage.value}</TextValue>;
};

interface PanelProps extends HTMLAttributes<HTMLDivElement> {}

const Panel: FC<PanelProps> = ({ children, ...attrs }) => {
	const theme = useTheme();

	return (
		<div
			css={{
				borderRadius: 8,
				border: `1px solid ${theme.palette.divider}`,
				backgroundColor: theme.palette.background.paper,
				display: "flex",
				flexDirection: "column",
			}}
			{...attrs}
		>
			{children}
		</div>
	);
};

const PanelHeader: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	return (
		<div css={{ padding: "20px 24px 24px" }} {...attrs}>
			{children}
		</div>
	);
};

const PanelTitle: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	return (
		<div css={{ fontSize: 14, fontWeight: 500 }} {...attrs}>
			{children}
		</div>
	);
};

const PanelContent: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	return (
		<div css={{ padding: "0 24px 24px", flex: 1 }} {...attrs}>
			{children}
		</div>
	);
};

const NoDataAvailable = (props: HTMLAttributes<HTMLDivElement>) => {
	const theme = useTheme();

	return (
		<div
			{...props}
			css={{
				fontSize: 13,
				color: theme.palette.text.secondary,
				textAlign: "center",
				height: "100%",
				display: "flex",
				alignItems: "center",
				justifyContent: "center",
			}}
		>
			No data available
		</div>
	);
};

const TextValue: FC<PropsWithChildren> = ({ children }) => {
	const theme = useTheme();

	return (
		<span>
			<span
				css={{
					color: theme.palette.text.secondary,
					weight: 600,
					marginRight: 2,
				}}
			>
				&quot;
			</span>
			{children}
			<span
				css={{
					color: theme.palette.text.secondary,
					weight: 600,
					marginLeft: 2,
				}}
			>
				&quot;
			</span>
		</span>
	);
};

function formatTime(seconds: number): string {
	let value: {
		amount: number;
		unit: "seconds" | "minutes" | "hours";
	} = {
		amount: seconds,
		unit: "seconds",
	};

	if (seconds >= 60 && seconds < 3600) {
		value = {
			amount: Math.floor(seconds / 60),
			unit: "minutes",
		};
	} else {
		value = {
			amount: seconds / 3600,
			unit: "hours",
		};
	}

	if (value.amount === 1) {
		const singularUnit = value.unit.slice(0, -1);
		return `${value.amount} ${singularUnit}`;
	}

	return `${value.amount.toLocaleString(undefined, {
		maximumFractionDigits: 1,
		minimumFractionDigits: 0,
	})} ${value.unit}`;
}

function toISOLocal(d: Date, offset: number) {
	return format(d, `yyyy-MM-dd'T'HH:mm:ss${formatOffset(offset)}`);
}

function formatOffset(offset: number): string {
	// A negative offset means that this is a positive timezone, e.g. GMT+2 = -120.
	const isPositiveTimezone = offset <= 0;
	const absoluteOffset = Math.abs(offset);
	const hours = Math.floor(absoluteOffset / 60);
	const minutes = Math.abs(offset) % 60;
	const formattedHours = `${isPositiveTimezone ? "+" : "-"}${String(
		hours,
	).padStart(2, "0")}`;
	const formattedMinutes = String(minutes).padStart(2, "0");
	return `${formattedHours}:${formattedMinutes}`;
}
