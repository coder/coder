import {
	CpuIcon,
	EllipsisIcon,
	GitBranchIcon,
	PencilIcon,
	PlusIcon,
	TrashIcon,
} from "lucide-react";
import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { SearchField } from "#/components/SearchField/SearchField";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { pageTitle } from "#/utils/page";
import { type DemoRunner, demoTemplates, getRunnerSnapshot } from "./demoData";
import { useRunners } from "./RunnersLayout";

const RunnersPage: React.FC = () => {
	const { runners, deleteRunner } = useRunners();
	const [search, setSearch] = useState("");
	const [deleting, setDeleting] = useState<DemoRunner>();
	const createButtonRef = useRef<HTMLAnchorElement>(null);
	const deleteTriggerRef = useRef<HTMLButtonElement | null>(null);
	const query = search.trim().toLowerCase();
	const filtered = runners.filter((runner) =>
		`${runner.label} ${runner.template} ${runner.preset}`
			.toLowerCase()
			.includes(query),
	);
	return (
		<Margins className="pb-12">
			<title>{pageTitle("Runners")}</title>
			<PageHeader
				actions={
					<Button asChild size="lg">
						<Link ref={createButtonRef} to="/runners/new">
							<PlusIcon />
							Create new runner
						</Link>
					</Button>
				}
			>
				<PageHeaderTitle>Runners</PageHeaderTitle>
				<PageHeaderSubtitle>
					Run GitHub Actions on ephemeral Coder workspaces.
				</PageHeaderSubtitle>
			</PageHeader>
			<div className="mb-6 flex flex-wrap items-center justify-between gap-4">
				<SearchField
					className="w-full sm:max-w-96"
					aria-label="Search runners"
					placeholder="Search by label or template..."
					value={search}
					onChange={setSearch}
				/>
				<span className="text-xs text-content-secondary">
					{filtered.length} {filtered.length === 1 ? "runner" : "runners"}
				</span>
			</div>
			<Table aria-label="Runners" className="min-w-190">
				<TableHeader>
					<TableRow>
						<TableHead className="w-[30%]">Runner</TableHead>
						<TableHead className="w-[27%]">Template</TableHead>
						<TableHead>In use</TableHead>
						<TableHead>Pool size</TableHead>
						<TableHead>Maximum</TableHead>
						<TableHead>
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{filtered.map((runner) => (
						<RunnerRow
							key={runner.id}
							runner={runner}
							onDelete={(trigger) => {
								deleteTriggerRef.current = trigger;
								setDeleting(runner);
							}}
						/>
					))}
					{filtered.length === 0 && (
						<TableRow>
							<TableCell colSpan={6}>
								<EmptyState
									message={
										query ? "No runners found" : "Create your first runner"
									}
									description={
										query
											? "Try a different runner label or template."
											: "Connect a runner label to a template to start running GitHub Actions in Coder."
									}
									cta={
										query ? (
											<Button variant="outline" onClick={() => setSearch("")}>
												Clear search
											</Button>
										) : (
											<Button asChild>
												<Link to="/runners/new">
													<PlusIcon />
													Create new runner
												</Link>
											</Button>
										)
									}
								/>
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
			<div className="mt-6 flex items-start gap-3 text-xs leading-relaxed text-content-secondary">
				<GitBranchIcon className="mt-0.5 size-4 shrink-0" />
				<p className="m-0">
					Use a runner label in your workflow&apos;s{" "}
					<code className="text-content-primary">runs-on</code> configuration.
					Each runner executes one job, then its workspace is automatically
					removed.
				</p>
			</div>
			<ConfirmDialog
				open={Boolean(deleting)}
				type="delete"
				title="Delete runner"
				confirmText="Delete runner"
				description={
					<>
						Delete <strong>{deleting?.label}</strong>? This removes its
						configuration from the runner list.
					</>
				}
				onClose={() => setDeleting(undefined)}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (deleteTriggerRef.current?.isConnected)
						deleteTriggerRef.current.focus();
					else createButtonRef.current?.focus();
				}}
				onConfirm={() => {
					if (deleting) deleteRunner(deleting.id);
					setDeleting(undefined);
				}}
			/>
		</Margins>
	);
};

type RunnerRowProps = {
	runner: DemoRunner;
	onDelete: (trigger: HTMLButtonElement | null) => void;
};

const RunnerRow: React.FC<RunnerRowProps> = ({ runner, onDelete }) => {
	const navigate = useNavigate();
	const triggerRef = useRef<HTMLButtonElement>(null);
	const snapshot = getRunnerSnapshot(runner);
	const template = demoTemplates.find(
		(template) => template.name === runner.template,
	);
	const href = `/runners/${runner.id}`;
	return (
		<TableRow hover onClick={() => navigate(href)}>
			<TableHead scope="row">
				<AvatarData
					avatar={
						<Avatar size="lg" variant="icon">
							<CpuIcon />
						</Avatar>
					}
					title={
						<Link
							to={href}
							className="text-inherit no-underline hover:underline"
							onClick={(event) => event.stopPropagation()}
						>
							{runner.label}
						</Link>
					}
					subtitle="Linux · x64"
				/>
			</TableHead>
			<TableCell>
				<div className="flex items-center gap-2.5">
					<Avatar src={template?.icon} fallback="CI" />
					<div className="flex flex-col gap-1">
						<span className="text-content-primary">
							{template?.displayName ?? runner.template}
						</span>
						<span>{runner.preset} preset</span>
					</div>
				</div>
			</TableCell>
			<TableCell>
				<div className="flex items-center gap-2">
					<span className="size-1.5 rounded-full bg-highlight-green" />
					<span className="text-content-primary">{snapshot.busy}</span>
					<span>in use</span>
				</div>
			</TableCell>
			<TableCell>
				<Badge variant="outline">
					{runner.poolSize === 0 ? "On demand" : `${runner.poolSize} ready`}
				</Badge>
			</TableCell>
			<TableCell>
				<span className="text-content-primary">{runner.maxRunners}</span>{" "}
				{runner.maxRunners === 1 ? "runner" : "runners"}
			</TableCell>
			<TableCell className="w-12">
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							ref={triggerRef}
							variant="subtle"
							size="icon"
							aria-label={`Actions for ${runner.label}`}
							onClick={(event) => event.stopPropagation()}
						>
							<EllipsisIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="end"
						onClick={(event) => event.stopPropagation()}
					>
						<DropdownMenuItem asChild>
							<Link to={`${href}/edit`}>
								<PencilIcon />
								Edit runner
							</Link>
						</DropdownMenuItem>
						<DropdownMenuSeparator />
						<DropdownMenuItem
							className="text-content-destructive"
							onSelect={() => onDelete(triggerRef.current)}
						>
							<TrashIcon />
							Delete runner
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</TableCell>
		</TableRow>
	);
};

export default RunnersPage;
