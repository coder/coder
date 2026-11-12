import {
	ArrowLeftIcon,
	CheckCircle2Icon,
	Clock3Icon,
	CpuIcon,
	PencilIcon,
	TerminalIcon,
	ZapIcon,
} from "lucide-react";
import { useState } from "react";
import { Link as RouterLink, useParams } from "react-router";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Link } from "#/components/Link/Link";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { pageTitle } from "#/utils/page";
import { humanDurationShort } from "#/utils/time";
import { demoTemplates, getRunnerJobs, getRunnerSnapshot } from "./demoData";
import { RunnerJobs } from "./RunnerJobs";
import { useRunners } from "./RunnersLayout";
import { RunnerUsageChart } from "./RunnerUsageChart";

const RunnerPage: React.FC = () => {
	const { id } = useParams();
	const { runners, now } = useRunners();
	const [range, setRange] = useState("1");
	const runner = runners.find((runner) => runner.id === id);
	if (!runner)
		return (
			<Margins>
				<EmptyState
					message="Runner not found"
					cta={
						<Button asChild variant="outline">
							<RouterLink to="/runners">Back to runners</RouterLink>
						</Button>
					}
				/>
			</Margins>
		);
	const days = Number(range);
	const snapshot = getRunnerSnapshot(runner);
	const jobs = getRunnerJobs(runner, now, days);
	const completed = jobs.filter((job) => job.status !== "In progress");
	const averageQueue = jobs.length
		? Math.round(
				jobs.reduce((total, job) => total + job.queueSeconds, 0) / jobs.length,
			)
		: 0;
	const template = demoTemplates.find(
		(template) => template.name === runner.template,
	);
	const workflow = `jobs:\n  build:\n    runs-on: ${JSON.stringify(runner.label)}\n    steps:\n      - uses: actions/checkout@v4\n      - run: echo "Hello from Coder"`;
	return (
		<Margins className="pb-12">
			<title>{pageTitle(runner.label, "Runners")}</title>
			<Button asChild variant="subtle" className="mt-8 -ml-3">
				<RouterLink to="/runners">
					<ArrowLeftIcon />
					Back to runners
				</RouterLink>
			</Button>
			<PageHeader
				className="py-8 [&_hgroup]:min-w-0"
				actions={
					<Button asChild variant="outline">
						<RouterLink to={`/runners/${runner.id}/edit`}>
							<PencilIcon />
							Edit runner
						</RouterLink>
					</Button>
				}
			>
				<PageHeaderTitle className="min-w-0 flex-wrap gap-3">
					<Avatar variant="icon" size="lg">
						<CpuIcon />
					</Avatar>
					<span className="min-w-0 break-all">{runner.label}</span>
					<Badge variant={snapshot.hasActivity ? "green" : "outline"}>
						{snapshot.hasActivity ? "Active" : "Ready for jobs"}
					</Badge>
				</PageHeaderTitle>
			</PageHeader>
			<div className="mb-6 grid grid-cols-2 gap-4 xl:grid-cols-4">
				<RunnerMetric
					title="Runners in use"
					value={String(snapshot.busy)}
					description={`${snapshot.starting} starting · ${runner.maxRunners} maximum`}
					icon={CpuIcon}
				/>
				<RunnerMetric
					title="Runners ready"
					value={String(snapshot.idle)}
					description={
						runner.poolSize === 0
							? "Provisioned on demand"
							: `${runner.poolSize} warm pool target`
					}
					icon={ZapIcon}
				/>
				<RunnerMetric
					title="Jobs completed"
					value={completed.length.toLocaleString("en-US")}
					description={days === 1 ? "Last 24 hours" : `Last ${days} days`}
					icon={CheckCircle2Icon}
				/>
				<RunnerMetric
					title="Average queue time"
					value={humanDurationShort(averageQueue * 1000)}
					description={
						jobs.length ? "Time before a job starts" : "No jobs in this period"
					}
					icon={Clock3Icon}
				/>
			</div>
			<div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_300px]">
				<div className="flex min-w-0 flex-col gap-6">
					<section className="rounded-lg border p-5 sm:p-6">
						<div className="mb-4 flex flex-wrap items-center justify-between gap-4">
							<div>
								<h2 className="m-0 text-base font-semibold">Runner usage</h2>
								<p className="mb-0 mt-1 text-xs text-content-secondary">
									Concurrent runners over time
								</p>
							</div>
							<Select value={range} onValueChange={setRange}>
								<SelectTrigger
									aria-label="Usage time range"
									className="h-9 w-40"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="1">Last 24 hours</SelectItem>
									<SelectItem value="7">Last 7 days</SelectItem>
									<SelectItem value="30">Last 30 days</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<RunnerUsageChart runner={runner} now={now} days={days} />
					</section>
					<RunnerJobs jobs={jobs} now={now} />
				</div>
				<aside className="grid gap-6 md:grid-cols-2 xl:grid-cols-1">
					<section className="rounded-lg border">
						<h2 className="m-0 border-b p-5 text-base font-semibold">
							Configuration
						</h2>
						<dl className="m-0 space-y-5 p-5 text-sm">
							<div>
								<dt className="mb-1.5 text-xs text-content-secondary">
									Runner label
								</dt>
								<dd className="m-0 break-all font-mono text-xs">
									{runner.label}
								</dd>
							</div>
							<div>
								<dt className="mb-1.5 text-xs text-content-secondary">
									Template
								</dt>
								<dd className="m-0 flex items-center gap-2">
									<Avatar src={template?.icon} size="sm" fallback="CI" />
									<Link asChild showExternalIcon={false} className="pl-0">
										<RouterLink to={`/templates/${runner.template}`}>
											{template?.displayName ?? runner.template}
										</RouterLink>
									</Link>
								</dd>
							</div>
							<div className="grid grid-cols-2 gap-4">
								<div>
									<dt className="mb-1.5 text-xs text-content-secondary">
										Preset
									</dt>
									<dd className="m-0">{runner.preset}</dd>
								</div>
								<div>
									<dt className="mb-1.5 text-xs text-content-secondary">
										Platform
									</dt>
									<dd className="m-0">Linux x64</dd>
								</div>
							</div>
							<div className="grid grid-cols-2 gap-4">
								<div>
									<dt className="mb-1.5 text-xs text-content-secondary">
										Pool size
									</dt>
									<dd className="m-0">
										{runner.poolSize === 0 ? "0 (on demand)" : runner.poolSize}
									</dd>
								</div>
								<div>
									<dt className="mb-1.5 text-xs text-content-secondary">
										Maximum
									</dt>
									<dd className="m-0">
										{runner.maxRunners}{" "}
										{runner.maxRunners === 1 ? "runner" : "runners"}
									</dd>
								</div>
							</div>
						</dl>
					</section>
					<section className="min-w-0 overflow-hidden rounded-lg border">
						<div className="flex items-center justify-between border-b px-5 py-3">
							<h2 className="m-0 flex items-center gap-2 text-sm font-semibold">
								<TerminalIcon className="size-4 text-content-secondary" />
								Use this runner
							</h2>
							<CopyButton text={workflow} label="Copy workflow YAML" />
						</div>
						<p className="m-0 px-5 pt-4 text-xs leading-relaxed text-content-secondary">
							Add this label to a GitHub Actions workflow to run jobs in Coder.
						</p>
						<pre className="m-0 overflow-x-auto p-5 text-xs leading-6 text-content-primary">
							<code>{workflow}</code>
						</pre>
					</section>
				</aside>
			</div>
		</Margins>
	);
};

type RunnerMetricProps = {
	title: string;
	value: string;
	description: string;
	icon: typeof CpuIcon;
};

const RunnerMetric: React.FC<RunnerMetricProps> = ({
	title,
	value,
	description,
	icon: Icon,
}) => (
	<div className="rounded-lg border p-5">
		<div className="flex items-center justify-between gap-2 text-xs text-content-secondary">
			<span>{title}</span>
			<Icon className="size-4 shrink-0" />
		</div>
		<div className="mb-2 mt-4 text-3xl font-semibold tracking-tight tabular-nums">
			{value}
		</div>
		<div className="text-xs text-content-secondary">{description}</div>
	</div>
);

export default RunnerPage;
