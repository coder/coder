import { useTheme } from "@emotion/react";
import LinearProgress from "@mui/material/LinearProgress";
import Link from "@mui/material/Link";
import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	insightsTemplate,
	insightsUserActivity,
	insightsUserLatency,
} from "api/queries/insights";
import type {
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
	HelpTooltipIconTrigger,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipArrow,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	CircleCheck as CircleCheckIcon,
	CircleXIcon,
	LinkIcon,
} from "lucide-react";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import {
	type FC,
	type HTMLAttributes,
	type PropsWithChildren,
	type ReactNode,
	useId,
} from "react";
import { useQuery } from "react-query";
import { type SetURLSearchParams, useSearchParams } from "react-router";
import { getLatencyColor } from "utils/latency";
import {
	addTime,
	formatDateTime,
	startOfDay,
	startOfHour,
	subtractTime,
} from "utils/time";
import { getTemplatePageTitle } from "../utils";
import { DateRange as DailyPicker, type DateRangeValue } from "./DateRange";
import { type InsightsInterval, IntervalMenu } from "./IntervalMenu";
import { lastWeeks } from "./utils";
import { numberOfWeeksOptions, WeekPicker } from "./WeekPicker";

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
	const templateInsights = useQuery(insightsTemplate(insightsFilter));
	const userLatency = useQuery(insightsUserLatency(commonFilters));
	const userActivity = useQuery(insightsUserActivity(commonFilters));

	return (
		<>
			<title>{getTemplatePageTitle("Insights", template)}</title>

			<TemplateInsightsPageView
				controls={
					<TemplateInsightsControls
						interval={interval}
						dateRange={dateRange}
						setDateRange={setDateRange}
						searchParams={searchParams}
						setSearchParams={setSearchParams}
					/>
				}
				templateInsights={templateInsights}
				userLatency={userLatency}
				userActivity={userActivity}
				interval={interval}
			/>
		</>
	);
}

interface TemplateInsightsControlsProps {
	interval: "day" | "week";
	dateRange: DateRangeValue;
	setDateRange: (value: DateRangeValue) => void;
	searchParams: URLSearchParams;
	setSearchParams: SetURLSearchParams;
}

export const TemplateInsightsControls: FC<TemplateInsightsControlsProps> = ({
	interval,
	dateRange,
	setDateRange,
	searchParams,
	setSearchParams,
}) => {
	return (
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
	);
};

const getDefaultInterval = (template: Template) => {
	const now = new Date();
	const templateCreateDate = new Date(template.created_at);
	const hasFiveWeeksOrMore = addTime(templateCreateDate, 5, "week") < now;
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
			startDate: startOfDay(subtractTime(today, 6, "day")),
			// Add one hour to endDate to include real-time data for today.
			endDate: addTime(startOfHour(today), 1, "hour"),
		};
	}

	return lastWeeks(DEFAULT_NUMBER_OF_WEEKS);
};

interface TemplateInsightsPageViewProps {
	templateInsights: {
		data: TemplateInsightsResponse | undefined;
		error: unknown;
	};
	userLatency: {
		data: UserLatencyInsightsResponse | undefined;
		error: unknown;
	};
	userActivity: {
		data: UserActivityInsightsResponse | undefined;
		error: unknown;
	};
	controls: ReactNode;
	interval: InsightsInterval;
}

export const TemplateInsightsPageView: FC<TemplateInsightsPageViewProps> = ({
	templateInsights,
	userLatency,
	userActivity,
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
					data={templateInsights.data?.interval_reports}
					error={templateInsights.error}
				/>
				<UsersLatencyPanel data={userLatency.data} error={userLatency.error} />
				<TemplateUsagePanel
					css={{ gridColumn: "span 2" }}
					data={templateInsights.data?.report?.apps_usage}
					error={templateInsights.error}
				/>
				<UsersActivityPanel
					data={userActivity.data}
					error={userActivity.error}
				/>
				<TemplateParametersUsagePanel
					css={{ gridColumn: "span 3" }}
					data={templateInsights.data?.report?.parameters_usage}
					error={templateInsights.error}
				/>
			</div>
		</>
	);
};

interface ActiveUsersPanelProps extends PanelProps {
	data: TemplateInsightsResponse["interval_reports"] | undefined;
	error: unknown;
	interval: InsightsInterval;
}

const ActiveUsersPanel: FC<ActiveUsersPanelProps> = ({
	data,
	error,
	interval,
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
				{!error && !data && <Loader css={{ height: "100%" }} />}
				{(error || data?.length === 0) && <NoDataAvailable error={error} />}
				{data && data.length > 0 && (
					<ActiveUserChart
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
	error: unknown;
}

const UsersLatencyPanel: FC<UsersLatencyPanelProps> = ({
	data,
	error,
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
						<HelpTooltipIconTrigger size="small" />
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
				{!error && !users && <Loader css={{ height: "100%" }} />}
				{(error || users?.length === 0) && <NoDataAvailable error={error} />}
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
	error: unknown;
}

const UsersActivityPanel: FC<UsersActivityPanelProps> = ({
	data,
	error,
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
						<HelpTooltipIconTrigger size="small" />
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
				{!error && !users && <Loader css={{ height: "100%" }} />}
				{(error || users?.length === 0) && <NoDataAvailable error={error} />}
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
	error: unknown;
}

const TemplateUsagePanel: FC<TemplateUsagePanelProps> = ({
	data,
	error,
	...panelProps
}) => {
	const theme = useTheme();
	// The API returns a row for each app, even if the user didn't use it.
	const validUsage = data
		?.filter((u) => u.seconds > 0)
		.sort((a, b) => b.seconds - a.seconds);
	const totalInSeconds =
		validUsage?.reduce((total, usage) => total + usage.seconds, 0) ?? 1;
	const usageColors = chroma
		.scale([theme.roles.success.fill.solid, theme.roles.warning.fill.solid])
		.mode("lch")
		.colors(validUsage?.length ?? 0);

	return (
		<Panel {...panelProps} css={{ overflowY: "auto" }}>
			<PanelHeader>
				<PanelTitle>App & IDE Usage</PanelTitle>
			</PanelHeader>
			<PanelContent>
				{!error && !data && <Loader css={{ height: "100%" }} />}
				{(error || validUsage?.length === 0) && (
					<NoDataAvailable error={error} />
				)}
				{validUsage && validUsage.length > 0 && (
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
									<Tooltip>
										<TooltipTrigger asChild>
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
										</TooltipTrigger>
										<TooltipContent>
											{Math.floor(percentage)}%
											<TooltipArrow className="fill-border" />
										</TooltipContent>
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
	error: unknown;
}

const TemplateParametersUsagePanel: FC<TemplateParametersUsagePanelProps> = ({
	data,
	error,
	...panelProps
}) => {
	const theme = useTheme();

	return (
		<Panel {...panelProps}>
			<PanelHeader>
				<PanelTitle>Parameters usage</PanelTitle>
			</PanelHeader>
			<PanelContent>
				{!error && !data && <Loader css={{ height: 200 }} />}
				{(error || data?.length === 0) && (
					<NoDataAvailable error={error} css={{ height: 200 }} />
				)}
				{data?.map((parameter, parameterIndex) => {
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
								"&:first-of-type": {
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
									<Tooltip>
										<TooltipTrigger asChild>
											<div>Count</div>
										</TooltipTrigger>
										<TooltipContent>
											The number of workspaces using this value
										</TooltipContent>
									</Tooltip>
								</ParameterUsageRow>
								{[...parameter.values]
									.sort((a, b) => b.count - a.count)
									.filter((usage) => filterOrphanValues(usage, parameter))
									.map((usage, usageIndex) => (
										<ParameterUsageRow key={`${parameterIndex}-${usageIndex}`}>
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
				<LinkIcon className="size-icon-xs text-content-link" />
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
						<CircleXIcon className="size-icon-xs text-content-destructive" />
						False
					</>
				) : (
					<>
						<CircleCheckIcon
							css={{
								color: theme.palette.success.light,
							}}
							className="size-icon-xs"
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

interface NoDataAvailableProps extends HTMLAttributes<HTMLDivElement> {
	error: unknown;
}

const NoDataAvailable: FC<NoDataAvailableProps> = ({ error, ...props }) => {
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
			{error
				? getErrorDetail(error) ||
					getErrorMessage(error, "Unable to fetch insights")
				: "No data available"}
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
	return formatDateTime(d, `YYYY-MM-DD[T]HH:mm:ss${formatOffset(offset)}`);
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
