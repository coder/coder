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
import { cn } from "utils/cn";
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
			<div className="flex items-center gap-2 mb-8">{controls}</div>
			<div className="grid gap-6 grid-cols-3 grid-rows-[440px_440px_auto]">
				<ActiveUsersPanel
					className="col-span-2"
					interval={interval}
					data={templateInsights.data?.interval_reports}
					error={templateInsights.error}
				/>
				<UsersLatencyPanel data={userLatency.data} error={userLatency.error} />
				<TemplateUsagePanel
					className="col-span-2"
					data={templateInsights.data?.report?.apps_usage}
					error={templateInsights.error}
				/>
				<UsersActivityPanel
					data={userActivity.data}
					error={userActivity.error}
				/>
				<TemplateParametersUsagePanel
					className="col-span-3"
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
			<PanelContent error={error} data={data}>
				<ActiveUserChart
					data={(data || []).map((d) => ({
						amount: d.active_users,
						date: d.start_time,
					}))}
				/>
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
	className,
	...panelProps
}) => {
	const theme = useTheme();
	return (
		<Panel {...panelProps} className={cn("overflow-y-auto", className)}>
			<PanelHeader>
				<PanelTitle className="flex items-center gap-2">
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
			<PanelContent error={error} data={data?.report.users}>
				{data?.report.users &&
					[...data.report.users]
						.sort((a, b) => b.latency_ms.p50 - a.latency_ms.p50)
						.map((row) => (
							<div
								key={row.user_id}
								className="flex justify-between items-center text-[14px] py-2"
							>
								<div className="flex items-center gap-3">
									<Avatar fallback={row.username} src={row.avatar_url} />
									<div className="font-medium">{row.username}</div>
								</div>
								<div
									className="text-right font-medium text-[13px]"
									css={{
										color: getLatencyColor(theme, row.latency_ms.p50),
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
	className,
	...panelProps
}) => {
	return (
		<Panel {...panelProps} className={cn("overflow-y-auto", className)}>
			<PanelHeader>
				<PanelTitle className="flex items-center gap-2">
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
			<PanelContent error={error} data={data?.report.users}>
				{data?.report.users &&
					[...data.report.users]
						.sort((a, b) => b.seconds - a.seconds)
						.map((row) => (
							<div
								key={row.user_id}
								className="flex justify-between items-center text-[14px] py-2"
							>
								<div className="flex items-center gap-3">
									<Avatar fallback={row.username} src={row.avatar_url} />
									<div className="font-medium">{row.username}</div>
								</div>
								<div className="text-right text-[13px] text-content-secondary">
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
	className,
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
		<Panel {...panelProps} className={cn("overflow-y-auto", className)}>
			<PanelHeader>
				<PanelTitle>App & IDE Usage</PanelTitle>
			</PanelHeader>
			<PanelContent error={error} data={validUsage}>
				{
					<div className="flex flex-col gap-6">
						{(validUsage || []).map((usage, i) => {
							const percentage = (usage.seconds / totalInSeconds) * 100;
							return (
								<div key={usage.slug} className="flex items-center gap-6">
									<div className="flex items-center gap-2">
										<div className="flex justify-center items-center w-5 h-5">
											<img
												src={usage.icon}
												alt=""
												className="h-full w-full object-contain"
											/>
										</div>
										<div className="text-[13px] font-medium w-[200px]">
											{usage.display_name}
										</div>
									</div>
									<Tooltip>
										<TooltipTrigger asChild>
											<LinearProgress
												value={percentage}
												variant="determinate"
												className="w-full h-2 bg-surface-quaternary"
												css={{
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
										className="text-[13px] shrink-0 leading-[1.5] text-content-secondary w-[120px]"
									>
										{formatTime(usage.seconds)}
										{usage.times_used > 0 && (
											<span className="text-[12px] text-content-disabled">
												Opened {usage.times_used.toLocaleString()}{" "}
												{usage.times_used === 1 ? "time" : "times"}
											</span>
										)}
									</Stack>
								</div>
							);
						})}
					</div>
				}
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
	return (
		<Panel {...panelProps}>
			<PanelHeader>
				<PanelTitle>Parameters usage</PanelTitle>
			</PanelHeader>
			<PanelContent error={error} data={data}>
				{data?.map((parameter, parameterIndex) => {
					const label =
						parameter.display_name !== ""
							? parameter.display_name
							: parameter.name;
					return (
						<div
							key={parameter.name}
							className="flex items-start gap-6 border-0 border-t border-solid border-surface-quaternary p-6 -mx-6"
							css={{
								"&:first-of-type": {
									borderTop: 0,
								},
							}}
						>
							<div className="flex-1">
								<div className="font-medium">{label}</div>
								<p className="text-[14px] m-0 text-content-secondary max-w-[400px]">
									{parameter.description}
								</p>
							</div>
							<div className="flex-1 text-[14px]" style={{ flexGrow: 2 }}>
								<ParameterUsageRow className="font-medium text-[13px] cursor-default text-content-secondary">
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
											<div className="text-right">{usage.count}</div>
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
	className,
	...attrs
}) => {
	return (
		<div
			{...attrs}
			className={cn("flex items-baseline justify-between py-1", className)}
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

	if (parameter.options) {
		const option = parameter.options.find((o) => o.value === usage.value)!;
		const icon = option.icon;
		const label = option.name;

		return (
			<div className="flex items-center gap-4">
				{icon && (
					<div className="leading-none w-4 h-4">
						<img
							alt=""
							src={icon}
							className="w-full h-full object-contain"
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
				className="flex items-center gap-[1px] text-content-primary"
			>
				<TextValue>{usage.value}</TextValue>
				<LinkIcon className="size-icon-xs text-content-link" />
			</Link>
		);
	}

	if (parameter.type === "list(string)") {
		const values = JSON.parse(usage.value) as string[];
		return (
			<div className="flex gap-2 flex-wrap">
				{values.map((v, i) => (
					<div
						key={i}
						className="rounded-full whitespace-nowrap bg-surface-quaternary py-0.5 px-3"
					>
						{v}
					</div>
				))}
			</div>
		);
	}

	if (parameter.type === "bool") {
		return (
			<div className="flex items-center gap-2">
				{usage.value === "false" ? (
					<>
						<CircleXIcon className="size-icon-xs text-content-destructive" />
						False
					</>
				) : (
					<>
						<CircleCheckIcon className="size-icon-xs text-content-success" />
						True
					</>
				)}
			</div>
		);
	}

	return <TextValue>{usage.value}</TextValue>;
};

interface PanelProps extends HTMLAttributes<HTMLDivElement> {}

const Panel: FC<PanelProps> = ({ children, className, ...attrs }) => {
	return (
		<div
			{...attrs}
			className={cn(
				"flex flex-col rounded-lg bg-surface-secondary border border-solid border-surface-quaternary",
				className,
			)}
		>
			{children}
		</div>
	);
};

const PanelHeader: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	className,
	...attrs
}) => {
	return (
		<div {...attrs} className={cn("p-6 pt-5", className)}>
			{children}
		</div>
	);
};

const PanelTitle: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	className,
	...attrs
}) => {
	return (
		<div {...attrs} className={cn("text-[14px] font-medium", className)}>
			{children}
		</div>
	);
};

interface PanelContentProps extends HTMLAttributes<HTMLDivElement> {
	error: unknown | undefined;
	data: readonly unknown[] | undefined;
}

const PanelContent: FC<PanelContentProps> = ({ error, data, children }) => {
	return (
		<div className="flex-1 px-6 pb-6">
			{!error && !data ? (
				<Loader className="h-full min-h-[200px]" />
			) : error || !data || data.length === 0 ? (
				<NoDataAvailable error={error} />
			) : (
				children
			)}
		</div>
	);
};

interface NoDataAvailableProps extends HTMLAttributes<HTMLDivElement> {
	error: unknown;
}

const NoDataAvailable: FC<NoDataAvailableProps> = ({ error, ...props }) => {
	return (
		<div
			{...props}
			className="flex justify-center items-center text-[13px] py-2 text-content-secondary text-center h-full min-h-[200px]"
		>
			{error
				? getErrorDetail(error) ||
					getErrorMessage(error, "Unable to fetch insights")
				: "No data available"}
		</div>
	);
};

const TextValue: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span>
			<span className="mr-0.5 text-content-secondary">&quot;</span>
			{children}
			<span className="ml-0.5 text-content-secondary">&quot;</span>
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
