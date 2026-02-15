import type { GetLicensesResponse } from "api/api";
import { Button } from "components/Button/Button";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "components/Chart/Chart";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { Link } from "components/Link/Link";
import { Spinner } from "components/Spinner/Spinner";
import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import {
	Area,
	CartesianGrid,
	ComposedChart,
	Line,
	XAxis,
	YAxis,
} from "recharts";
import { docs } from "utils/docs";
import { formatDate } from "utils/time";

const chartConfig = {
	users: {
		label: "Users",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;

/**
 * Returns the effective seat limit on a given date by finding the
 * maximum user_limit across all licenses that were active on that
 * date. This mirrors the backend's AddFeature/Compare logic where
 * the highest limit wins.
 */
export function getEffectiveSeatLimit(
	date: Date,
	licenses: GetLicensesResponse[],
): number | undefined {
	const limits = licenses
		.filter((l) => {
			const nbf = l.claims.nbf ? new Date(l.claims.nbf * 1000) : new Date(0);
			const expires = new Date(l.claims.license_expires * 1000);
			return nbf <= date && expires >= date;
		})
		.map((l) => l.claims.features.user_limit)
		.filter((v): v is number => v != null && v > 0);
	return limits.length > 0 ? Math.max(...limits) : undefined;
}

type LicenseSeatConsumptionChartProps = {
	data:
		| {
				date: string;
				users: number;
				limit: number | undefined;
		  }[]
		| undefined;
};

export const LicenseSeatConsumptionChart: FC<
	LicenseSeatConsumptionChartProps
> = ({ data }) => {
	const hasLimit = data?.some((d) => d.limit != null);
	return (
		<section className="border border-solid rounded">
			<div className="p-4">
				<Collapsible>
					<header className="flex flex-col gap-2 items-start">
						<h3 className="text-md m-0 font-medium">
							License seat consumption
						</h3>

						<CollapsibleTrigger asChild>
							<Button
								className={`
									h-auto p-0 border-0 bg-transparent font-medium text-content-secondary
									hover:bg-transparent hover:text-content-primary
									[&[data-state=open]_svg]:rotate-90
								`}
							>
								<ChevronRightIcon />
								How we calculate license seat consumption
							</Button>
						</CollapsibleTrigger>
					</header>

					<CollapsibleContent
						className={`
							pt-2 pl-7 pr-5 space-y-4 font-medium max-w-[720px]
							text-sm text-content-secondary
							[&_p]:m-0 [&_ul]:m-0 [&_ul]:p-0 [&_ul]:list-none
						`}
					>
						<p>
							Licenses are consumed based on the status of user accounts. Only
							Active user accounts are consuming license seats.
						</p>
						<ul>
							<li className="flex items-center gap-2">
								<div className="rounded-[2px] bg-highlight-green size-3 inline-block">
									<span className="sr-only">
										Legend for active users in the chart
									</span>
								</div>
								The user was active at least once during the last 90 days.
							</li>
							<li className="flex items-center gap-2">
								<div className="size-3 inline-flex items-center justify-center">
									<span className="sr-only">
										Legend for license seat limit in the chart
									</span>
									<div className="w-full border-b-1 border-t-1 border-dashed border-content-disabled" />
								</div>
								Current license seat limit, or the maximum number of allowed
								Active accounts.
							</li>
						</ul>
						<div>
							You might also check:
							<ul>
								<li>
									<Link asChild>
										<RouterLink to="/audit">Activity Audit</RouterLink>
									</Link>
								</li>
								<li>
									<Link asChild>
										<RouterLink to="/deployment/overview">
											Daily user activity
										</RouterLink>
									</Link>
								</li>
								<li>
									<Link
										href={docs("/admin/users#user-status")}
										target="_blank"
										rel="noreferrer"
									>
										More details on user account statuses
									</Link>
								</li>
							</ul>
						</div>
					</CollapsibleContent>
				</Collapsible>
			</div>

			<div className="p-6 border-0 border-t border-solid">
				<div className="h-64">
					{data ? (
						data.length > 0 ? (
							<ChartContainer
								config={chartConfig}
								className="aspect-auto h-full"
							>
								<ComposedChart
									accessibilityLayer
									data={data}
									margin={{
										top: 5,
										right: 5,
										left: 0,
									}}
								>
									<CartesianGrid vertical={false} />
									<XAxis
										dataKey="date"
										tickLine={false}
										tickMargin={12}
										minTickGap={24}
										tickFormatter={(value: string) =>
											formatDate(new Date(value), {
												month: "short",
												day: "numeric",
												year: undefined,
												hour: undefined,
												minute: undefined,
												second: undefined,
											})
										}
									/>
									<YAxis
										domain={[0, "auto"]}
										tickLine={false}
										axisLine={false}
										tickMargin={12}
										tickFormatter={(value: number) => {
											return value === 0 ? "" : value.toLocaleString();
										}}
									/>
									<ChartTooltip
										cursor={false}
										content={({ content: _, ...props }) => (
											<ChartTooltipContent
												{...props}
												payload={props.payload?.filter(
													(p) => p.dataKey !== "limit",
												)}
												className="font-medium text-content-secondary"
												labelFormatter={(_, p) => {
													const item = p[0];
													const limit = item.payload?.limit;
													return (
														<div className="flex flex-col gap-1 text-content-primary">
															<div className="flex items-center gap-4 flex-1 justify-between">
																<div className="flex items-center gap-2">
																	<div className="size-3 rounded-sm bg-[var(--color-users)]" />
																	<span>Active Users</span>
																</div>
																<span>{item.value}</span>
															</div>
															{limit != null && (
																<div className="flex items-center gap-4 flex-1 justify-between">
																	<div className="flex items-center gap-2">
																		<div className="w-3 border-b-1 border-t-1 border-dashed border-content-disabled"></div>
																		<span>Seat Limit</span>
																	</div>
																	<span>{limit}</span>
																</div>
															)}
														</div>
													);
												}}
												formatter={(_v, _n, item) => {
													const date = new Date(item.payload.date);
													return date.toLocaleString(undefined, {
														month: "long",
														day: "2-digit",
													});
												}}
											/>
										)}
									/>
									<defs>
										<linearGradient id="fillUsers" x1="0" y1="0" x2="0" y2="1">
											<stop
												offset="5%"
												stopColor="var(--color-users)"
												stopOpacity={0.8}
											/>
											<stop
												offset="95%"
												stopColor="var(--color-users)"
												stopOpacity={0.1}
											/>
										</linearGradient>
									</defs>

									<Area
										dataKey="users"
										type="linear"
										fill="url(#fillUsers)"
										fillOpacity={0.4}
										stroke="var(--color-users)"
										stackId="a"
									/>

									{hasLimit && (
										<Line
											dataKey="limit"
											type="stepAfter"
											stroke="hsl(var(--content-disabled))"
											strokeDasharray="5 5"
											strokeWidth={1}
											dot={false}
											isAnimationActive={false}
											tooltipType="none"
										/>
									)}
								</ComposedChart>
							</ChartContainer>
						) : (
							<div
								className={`
									w-full h-full flex items-center justify-center
									text-content-secondary text-sm font-medium
								`}
							>
								No data available
							</div>
						)
					) : (
						<div className="w-full h-full flex items-center justify-center">
							<Spinner loading />
						</div>
					)}
				</div>
			</div>
		</section>
	);
};
