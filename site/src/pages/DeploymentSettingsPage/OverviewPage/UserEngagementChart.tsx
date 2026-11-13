import { cn } from "cn";
import { ChevronRightIcon } from "lucide-react";
import { useId } from "react";
import { Link as RouterLink } from "react-router";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { Button } from "#/components/Button/Button";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
	getDailyChartDescription,
} from "#/components/Chart/Chart";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { formatDate } from "#/utils/time";

const chartConfig = {
	users: {
		label: "Users",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;

type UserEngagementDay = {
	date: string;
	users: number;
};

type UserEngagementChartProps = {
	data: UserEngagementDay[] | undefined;
};

export const UserEngagementChart: React.FC<UserEngagementChartProps> = ({
	data,
}) => {
	const titleId = useId();

	return (
		<section className="border rounded">
			<div className="p-4">
				<Collapsible>
					<header className="flex flex-col gap-2 items-start">
						<h3 className="text-md m-0 font-medium" id={titleId}>
							User Engagement
						</h3>

						<CollapsibleTrigger asChild>
							<DisclosureTrigger>
								How we calculate engaged users
							</DisclosureTrigger>
						</CollapsibleTrigger>
					</header>

					<CollapsibleContent
						className={cn(
							"pt-2 pl-7 pr-5 space-y-4 font-medium max-w-180",
							"[&_p]:m-0 [&_p]:text-sm [&_p]:text-content-secondary",
						)}
					>
						<p>
							A user is considered "engaged" if they initiate a connection to
							their workspace via apps, web terminal, or SSH. The graph displays
							the daily count of unique users who engaged at least once, with
							additional insights available through the{" "}
							<Link size="sm" asChild>
								<RouterLink to="/audit">Activity Audit</RouterLink>
							</Link>{" "}
							and{" "}
							<Link size="sm" asChild>
								<RouterLink to="/deployment/licenses">
									License Consumption
								</RouterLink>
							</Link>{" "}
							tools.
						</p>
					</CollapsibleContent>
				</Collapsible>
			</div>

			<div className="p-6 border-t">
				<div className="h-64">
					{data ? (
						data.length > 0 ? (
							<ChartContainer
								config={chartConfig}
								className="aspect-auto h-full"
							>
								<AreaChart
									accessibilityLayer
									role="group"
									aria-roledescription="chart"
									aria-labelledby={titleId}
									desc={`${getDailyChartDescription(
										"Daily engaged users",
										data.map((d) => d.date),
									)} Expand the data table below the chart to read every day.`}
									data={data}
									margin={{
										top: 10,
										left: 0,
										right: 0,
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
										dataKey="users"
										tickLine={false}
										axisLine={false}
										tickMargin={12}
										tickFormatter={(value: number) => {
											return value === 0 ? "" : value.toLocaleString();
										}}
									/>
									<ChartTooltip
										cursor={false}
										content={(props) => (
											<ChartTooltipContent
												{...props}
												className="font-medium text-content-secondary"
												labelClassName="text-content-primary"
												labelFormatter={(_, p) => {
													const item = p[0];
													return `${item.value} users`;
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
										isAnimationActive={false}
										dataKey="users"
										type="linear"
										fill="url(#fillUsers)"
										fillOpacity={0.4}
										stroke="var(--color-users)"
										stackId="a"
									/>
								</AreaChart>
							</ChartContainer>
						) : (
							<div
								className={cn(
									"size-full flex items-center justify-center",
									"text-content-secondary text-sm font-medium",
								)}
							>
								No data available
							</div>
						)
					) : (
						<div className="size-full flex items-center justify-center">
							<Spinner loading />
						</div>
					)}
				</div>

				{data && data.length > 0 && <UserEngagementTable data={data} />}
			</div>
		</section>
	);
};

const DisclosureTrigger: React.FC<React.ComponentProps<typeof Button>> = ({
	children,
	className,
	...props
}) => (
	<Button
		className={cn(
			"h-auto p-0 bg-transparent font-medium text-content-secondary",
			"hover:bg-transparent hover:text-content-primary",
			"[&[data-state=open]_svg]:rotate-90",
			className,
		)}
		{...props}
	>
		<ChevronRightIcon />
		{children}
	</Button>
);

type UserEngagementTableProps = {
	data: UserEngagementDay[];
};

// The chart's long description (WCAG 1.1.1): every value it plots, as text.
const UserEngagementTable: React.FC<UserEngagementTableProps> = ({ data }) => (
	<Collapsible className="mt-4">
		<CollapsibleTrigger asChild>
			<DisclosureTrigger>Data table</DisclosureTrigger>
		</CollapsibleTrigger>
		<CollapsibleContent className="pt-4">
			<Table aria-label="Daily engaged users">
				<TableHeader>
					<TableRow>
						<TableHead>Date</TableHead>
						<TableHead>Engaged users</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{data.map((d) => (
						<TableRow key={d.date}>
							<TableHead scope="row" className="border-t font-medium">
								{formatDate(new Date(d.date), {
									hour: undefined,
									minute: undefined,
									second: undefined,
								})}
							</TableHead>
							<TableCell>{d.users.toLocaleString("en-US")}</TableCell>
						</TableRow>
					))}
				</TableBody>
			</Table>
		</CollapsibleContent>
	</Collapsible>
);
