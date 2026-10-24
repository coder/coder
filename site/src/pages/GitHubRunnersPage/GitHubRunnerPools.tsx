import { MoreHorizontalIcon, PlusIcon } from "lucide-react";
import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	createGitHubRunnerPool,
	deleteGitHubRunnerPool,
	githubRunnerPools,
	updateGitHubRunnerPool,
} from "#/api/queries/githubRunners";
import type { GitHubRunnerPool, Organization } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";
import { GitHubRunnerAllocations } from "./GitHubRunnerAllocations";
import { RunnerPoolForm } from "./RunnerPoolForm";

type GitHubRunnerPoolsProps = {
	organization: Organization;
	permissions: OrganizationPermissions;
};

/** Holds dialog snapshots so status polling cannot overwrite capacity edits. */
export const GitHubRunnerPools: React.FC<GitHubRunnerPoolsProps> = ({
	organization,
	permissions,
}) => {
	const { user } = useAuthenticated();
	const queryClient = useQueryClient();
	const poolsQuery = useQuery(githubRunnerPools(organization.id));
	const createMutation = useMutation(
		createGitHubRunnerPool(queryClient, organization.id),
	);
	const updateMutation = useMutation(
		updateGitHubRunnerPool(queryClient, organization.id),
	);
	const deleteMutation = useMutation(
		deleteGitHubRunnerPool(queryClient, organization.id),
	);
	const [dialog, setDialog] = useState<
		{ type: "create" } | { type: "capacity" | "delete"; pool: GitHubRunnerPool }
	>();
	const [selectedPool, setSelectedPool] = useState<string>();
	const triggerRef = useRef<HTMLButtonElement | null>(null);
	const closeDialog = () => {
		setDialog(undefined);
		createMutation.reset();
		updateMutation.reset();
		deleteMutation.reset();
	};
	const selectedPoolData = poolsQuery.data?.find(
		(pool) => pool.id === selectedPool,
	);
	const formPending = createMutation.isPending || updateMutation.isPending;

	return (
		<div className="space-y-6">
			<PageHeader
				actions={
					permissions.createGitHubRunnerPool && (
						<Button
							onClick={(event) => {
								triggerRef.current = event.currentTarget;
								setDialog({ type: "create" });
							}}
						>
							<PlusIcon />
							Create pool
						</Button>
					)
				}
			>
				<PageHeaderTitle>GitHub runners</PageHeaderTitle>
				<PageHeaderSubtitle>
					Run GitHub Actions jobs in disposable Coder workspaces.
				</PageHeaderSubtitle>
			</PageHeader>
			{poolsQuery.error && (
				<div className="space-y-3">
					<ErrorAlert error={poolsQuery.error} />
					<Button variant="outline" onClick={() => void poolsQuery.refetch()}>
						Retry pools
					</Button>
				</div>
			)}
			{!dialog && updateMutation.error && (
				<ErrorAlert error={updateMutation.error} />
			)}
			{!poolsQuery.data && !poolsQuery.error && <Loader />}
			{poolsQuery.data?.length === 0 && (
				<EmptyState
					message="No runner pools yet"
					description="Create a pool to connect a GitHub repository or organization to a Coder runner template."
				/>
			)}
			{poolsQuery.data && poolsQuery.data.length > 0 && (
				<Table aria-label="GitHub runner pools">
					<TableHeader>
						<TableRow>
							<TableHead>Pool</TableHead>
							<TableHead>Status</TableHead>
							<TableHead>Assigned jobs</TableHead>
							<TableHead>Capacity</TableHead>
							<TableHead>
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{poolsQuery.data.map((pool) => (
							<PoolRow
								key={pool.id}
								pool={pool}
								permissions={permissions}
								isUpdating={updateMutation.isPending}
								onManageFocus={(button) => {
									triggerRef.current = button;
								}}
								onViewRunners={() => setSelectedPool(pool.id)}
								onToggleState={() => {
									updateMutation.reset();
									updateMutation.mutate({
										pool: pool.id,
										request: {
											state: pool.state === "active" ? "paused" : "active",
										},
									});
								}}
								onEditCapacity={() => {
									updateMutation.reset();
									setDialog({ type: "capacity", pool });
								}}
								onDelete={() => {
									deleteMutation.reset();
									setDialog({ type: "delete", pool });
								}}
							/>
						))}
					</TableBody>
				</Table>
			)}
			{selectedPoolData && (
				<GitHubRunnerAllocations
					organization={organization.id}
					pool={selectedPoolData}
				/>
			)}
			{dialog && dialog.type !== "delete" && (
				<Dialog
					open
					onOpenChange={(open) => {
						if (!open && !formPending) closeDialog();
					}}
				>
					<DialogContent
						className="max-w-2xl"
						onCloseAutoFocus={(event) => {
							event.preventDefault();
							triggerRef.current?.focus();
						}}
					>
						<DialogHeader>
							<DialogTitle>
								{dialog.type === "create"
									? "Create runner pool"
									: `Edit capacity for ${dialog.pool.name}`}
							</DialogTitle>
							<DialogDescription>
								{dialog.type === "create"
									? "Connect GitHub to a runner template. Each job gets a fresh workspace."
									: "Set the maximum number of simultaneous runner workspaces. Running jobs are allowed to finish when capacity is reduced."}
							</DialogDescription>
						</DialogHeader>
						<RunnerPoolForm
							organization={organization.id}
							defaultOwner={user}
							pool={dialog.type === "capacity" ? dialog.pool : undefined}
							error={
								dialog.type === "create"
									? createMutation.error
									: updateMutation.error
							}
							isPending={formPending}
							onCancel={closeDialog}
							onSubmit={(request) => {
								if (dialog.type === "create")
									createMutation.mutate(request, { onSuccess: closeDialog });
								else
									updateMutation.mutate(
										{
											pool: dialog.pool.id,
											request: { max_runners: request.max_runners },
										},
										{ onSuccess: closeDialog },
									);
							}}
						/>
					</DialogContent>
				</Dialog>
			)}
			{dialog?.type === "delete" && (
				<ConfirmDialog
					open
					type="delete"
					title={`Delete ${dialog.pool.name}?`}
					onCloseAutoFocus={(event) => {
						event.preventDefault();
						triggerRef.current?.focus();
					}}
					confirmText="Drain and delete"
					confirmLoading={deleteMutation.isPending}
					onClose={() => {
						if (!deleteMutation.isPending) closeDialog();
					}}
					onConfirm={() =>
						deleteMutation.mutate(dialog.pool.id, { onSuccess: closeDialog })
					}
					description={
						<>
							<p>
								This stops acquiring new jobs. Running jobs finish before Coder
								deletes the pool and its GitHub scale set. A draining pool
								cannot be resumed.
							</p>
							{deleteMutation.error && (
								<ErrorAlert error={deleteMutation.error} />
							)}
						</>
					}
				/>
			)}
		</div>
	);
};

type PoolRowProps = {
	pool: GitHubRunnerPool;
	permissions: OrganizationPermissions;
	isUpdating: boolean;
	onManageFocus: (button: HTMLButtonElement) => void;
	onViewRunners: () => void;
	onToggleState: () => void;
	onEditCapacity: () => void;
	onDelete: () => void;
};

const PoolRow: React.FC<PoolRowProps> = ({
	pool,
	permissions,
	isUpdating,
	onManageFocus,
	onViewRunners,
	onToggleState,
	onEditCapacity,
	onDelete,
}) => (
	<TableRow>
		<TableCell>
			<Button
				variant="subtle"
				className="px-0 h-auto min-w-0 text-content-primary"
				onClick={onViewRunners}
			>
				{pool.name}
			</Button>
			<a
				className="block mt-1 text-xs text-content-secondary"
				href={pool.github_config_url}
				target="_blank"
				rel="noreferrer"
			>
				{pool.github_config_url}
				<span className="sr-only"> (opens in new tab)</span>
			</a>
			{pool.last_error && (
				<p className="mb-0 max-w-lg text-xs text-content-destructive break-words">
					{pool.last_error}
				</p>
			)}
		</TableCell>
		<TableCell>
			<Badge
				variant={
					pool.state === "active"
						? "green"
						: pool.state === "draining"
							? "warning"
							: "outline"
				}
			>
				{pool.state}
			</Badge>
		</TableCell>
		<TableCell>{pool.assigned_jobs}</TableCell>
		<TableCell>{pool.max_runners}</TableCell>
		<TableCell className="text-right">
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						size="icon"
						variant="subtle"
						aria-label={`Manage ${pool.name}`}
						onFocus={(event) => onManageFocus(event.currentTarget)}
						onClick={(event) => onManageFocus(event.currentTarget)}
					>
						<MoreHorizontalIcon />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem onSelect={onViewRunners}>
						View runners
					</DropdownMenuItem>
					{permissions.updateGitHubRunnerPool &&
						(pool.state === "active" || pool.state === "paused") && (
							<>
								<DropdownMenuItem
									disabled={isUpdating}
									onSelect={onToggleState}
								>
									{pool.state === "active" ? "Pause" : "Resume"}
								</DropdownMenuItem>
								<DropdownMenuItem onSelect={onEditCapacity}>
									Edit capacity
								</DropdownMenuItem>
							</>
						)}
					{permissions.deleteGitHubRunnerPool &&
						pool.state !== "deleted" &&
						pool.state !== "draining" && (
							<DropdownMenuItem onSelect={onDelete}>
								Drain and delete
							</DropdownMenuItem>
						)}
				</DropdownMenuContent>
			</DropdownMenu>
		</TableCell>
	</TableRow>
);
