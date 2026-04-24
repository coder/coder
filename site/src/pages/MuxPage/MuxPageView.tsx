import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import type { Workspace } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Link } from "#/components/Link/Link";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { useAppLink } from "#/modules/apps/useAppLink";
import { DashboardFullPage } from "#/modules/dashboard/DashboardLayout";
import { pageTitle } from "#/utils/page";
import { MuxIframe } from "./MuxIframe";
import type { MuxAppCandidate } from "./muxApps";

type MuxPageViewProps = {
	isLoading: boolean;
	error: unknown;
	ownedWorkspaceCount: number | undefined;
	muxWorkspaces: readonly Workspace[];
	selectedWorkspace: Workspace | undefined;
	selectedMuxCandidate: MuxAppCandidate | undefined;
	isLaunched: boolean;
	onSelectWorkspace: (workspaceId: string | undefined) => void;
	onLaunchWorkspace: () => void;
	onChangeWorkspace: () => void;
};

export const MuxPageView: FC<MuxPageViewProps> = ({
	isLoading,
	error,
	ownedWorkspaceCount,
	muxWorkspaces,
	selectedWorkspace,
	selectedMuxCandidate,
	isLaunched,
	onSelectWorkspace,
	onLaunchWorkspace,
	onChangeWorkspace,
}) => {
	const resolvedPageTitle = selectedWorkspace
		? pageTitle(`Mux - ${selectedWorkspace.name}`)
		: pageTitle("Mux");

	return (
		<DashboardFullPage>
			<title>{resolvedPageTitle}</title>
			{isLoading ? (
				<MuxCenteredPanel>
					<Loader label="Loading workspaces" />
				</MuxCenteredPanel>
			) : error ? (
				<MuxCenteredPanel>
					<ErrorAlert error={error} />
				</MuxCenteredPanel>
			) : muxWorkspaces.length === 0 ? (
				<MuxCenteredPanel>
					<MuxEmptyState ownedWorkspaceCount={ownedWorkspaceCount} />
				</MuxCenteredPanel>
			) : isLaunched ? (
				<MuxLaunchedView
					selectedWorkspace={selectedWorkspace}
					selectedMuxCandidate={selectedMuxCandidate}
					onChangeWorkspace={onChangeWorkspace}
				/>
			) : (
				<MuxLauncher
					muxWorkspaces={muxWorkspaces}
					selectedWorkspace={selectedWorkspace}
					selectedMuxCandidate={selectedMuxCandidate}
					onSelectWorkspace={onSelectWorkspace}
					onLaunchWorkspace={onLaunchWorkspace}
				/>
			)}
		</DashboardFullPage>
	);
};

type MuxCenteredPanelProps = {
	children: ReactNode;
};

const MuxCenteredPanel: FC<MuxCenteredPanelProps> = ({ children }) => {
	return (
		<Margins className="flex min-h-0 flex-1 items-center justify-center py-10">
			<div className="w-full max-w-md rounded-lg border border-solid border-border bg-surface-primary p-6 shadow-sm">
				{children}
			</div>
		</Margins>
	);
};

type MuxLauncherProps = {
	muxWorkspaces: readonly Workspace[];
	selectedWorkspace: Workspace | undefined;
	selectedMuxCandidate: MuxAppCandidate | undefined;
	onSelectWorkspace: (workspaceId: string | undefined) => void;
	onLaunchWorkspace: () => void;
};

const MuxLauncher: FC<MuxLauncherProps> = ({
	muxWorkspaces,
	selectedWorkspace,
	selectedMuxCandidate,
	onSelectWorkspace,
	onLaunchWorkspace,
}) => {
	const canLaunch =
		selectedWorkspace?.latest_build.status === "running" &&
		selectedMuxCandidate !== undefined;
	const workspacePath = selectedWorkspace
		? `/@${selectedWorkspace.owner_name}/${selectedWorkspace.name}`
		: undefined;
	const helperLabel = !selectedWorkspace
		? "Select a workspace"
		: selectedWorkspace.latest_build.status === "stopped"
			? "Start workspace to launch"
			: selectedMuxCandidate === undefined
				? "Mux app unavailable"
				: undefined;

	return (
		<MuxCenteredPanel>
			<div className="flex flex-col items-center text-center">
				<h1 className="m-0 text-3xl font-medium text-content-primary">Mux</h1>
				<p className="m-0 mt-2 max-w-sm text-sm text-content-secondary">
					Open the Mux app from one of your workspaces.
				</p>

				<div className="mt-6 flex w-full max-w-sm flex-col items-stretch gap-4 text-left">
					<WorkspaceSelect
						muxWorkspaces={muxWorkspaces}
						selectedWorkspace={selectedWorkspace}
						onSelectWorkspace={onSelectWorkspace}
					/>

					<div className="flex flex-col gap-2 text-center">
						{selectedWorkspace?.latest_build.status === "stopped" &&
						workspacePath ? (
							<Button asChild className="w-full">
								<RouterLink to={workspacePath}>Open workspace</RouterLink>
							</Button>
						) : (
							<Button
								className="w-full"
								disabled={!canLaunch}
								onClick={onLaunchWorkspace}
								aria-describedby={helperLabel ? "mux-launch-helper" : undefined}
							>
								Launch
							</Button>
						)}
						{helperLabel ? (
							<p
								id="mux-launch-helper"
								className="m-0 text-xs text-content-secondary"
							>
								{helperLabel}
							</p>
						) : null}
					</div>
				</div>
			</div>
		</MuxCenteredPanel>
	);
};

type WorkspaceSelectProps = {
	muxWorkspaces: readonly Workspace[];
	selectedWorkspace: Workspace | undefined;
	onSelectWorkspace: (workspaceId: string | undefined) => void;
};

const WorkspaceSelect: FC<WorkspaceSelectProps> = ({
	muxWorkspaces,
	selectedWorkspace,
	onSelectWorkspace,
}) => {
	const selectedOption = selectedWorkspace
		? {
				label: `${selectedWorkspace.owner_name}/${selectedWorkspace.name}`,
				value: selectedWorkspace.id,
			}
		: undefined;

	return (
		<div className="flex w-full flex-col gap-2">
			<span
				id="mux-workspace-select-label"
				className="text-sm font-medium text-content-primary"
			>
				Select workspace
			</span>
			<div className="flex w-full">
				<Combobox
					value={selectedWorkspace?.id}
					onValueChange={onSelectWorkspace}
				>
					<ComboboxTrigger asChild>
						<ComboboxButton
							aria-labelledby="mux-workspace-select-label"
							placeholder="Choose a workspace"
							selectedOption={selectedOption}
							width={320}
						/>
					</ComboboxTrigger>
					<ComboboxContent align="center" className="w-80 max-w-[90vw]">
						<ComboboxInput placeholder="Search workspaces" />
						<ComboboxList>
							<ComboboxEmpty>No workspaces found.</ComboboxEmpty>
							{muxWorkspaces.map((workspace) => (
								<WorkspaceOption key={workspace.id} workspace={workspace} />
							))}
						</ComboboxList>
					</ComboboxContent>
				</Combobox>
			</div>
		</div>
	);
};

type MuxLaunchedViewProps = {
	selectedWorkspace: Workspace | undefined;
	selectedMuxCandidate: MuxAppCandidate | undefined;
	onChangeWorkspace: () => void;
};

const MuxLaunchedView: FC<MuxLaunchedViewProps> = ({
	selectedWorkspace,
	selectedMuxCandidate,
	onChangeWorkspace,
}) => {
	return (
		<section
			aria-label="Mux app"
			className="relative min-h-0 flex-1 overflow-hidden bg-surface-primary [&>div>div:first-child]:hidden"
		>
			{selectedWorkspace && selectedMuxCandidate ? (
				<>
					<MuxIframe
						workspace={selectedWorkspace}
						candidate={selectedMuxCandidate}
					/>
					<MuxLaunchedControls
						workspace={selectedWorkspace}
						candidate={selectedMuxCandidate}
						onChangeWorkspace={onChangeWorkspace}
					/>
				</>
			) : selectedWorkspace ? (
				<MissingSelectedApp />
			) : (
				<EmptyState
					isCompact
					className="h-full"
					message="Choose a workspace"
					description="Select a workspace to open its Mux app."
				/>
			)}
		</section>
	);
};

type MuxLaunchedControlsProps = {
	workspace: Workspace;
	candidate: MuxAppCandidate;
	onChangeWorkspace: () => void;
};

const MuxLaunchedControls: FC<MuxLaunchedControlsProps> = ({
	workspace,
	candidate,
	onChangeWorkspace,
}) => {
	const { agent, app } = candidate;
	const link = useAppLink(app, { agent, workspace });

	return (
		<div className="absolute right-3 top-3 z-10 flex items-center gap-2 rounded-md border border-solid border-border bg-surface-primary p-1 shadow-sm">
			<Button variant="subtle" size="sm" onClick={onChangeWorkspace}>
				Change workspace
			</Button>
			<Link
				href={link.href}
				target="_blank"
				rel="noreferrer"
				onClick={link.onClick}
				size="sm"
				className="px-1"
			>
				Open in new tab
			</Link>
		</div>
	);
};

type WorkspaceOptionProps = {
	workspace: Workspace;
};

const WorkspaceOption: FC<WorkspaceOptionProps> = ({ workspace }) => {
	const statusHint =
		workspace.latest_build.status === "stopped"
			? "Stopped, start required"
			: "Running";

	return (
		<ComboboxItem
			value={workspace.id}
			keywords={[workspace.name, workspace.owner_name, statusHint]}
			className="items-start gap-3 px-4 py-3"
		>
			<div className="min-w-0 flex-1">
				<p className="m-0 truncate text-sm font-medium text-content-primary">
					{workspace.name}
				</p>
				<p className="m-0 truncate text-xs text-content-secondary">
					{workspace.owner_name}, {statusHint}
				</p>
			</div>
		</ComboboxItem>
	);
};

type MuxEmptyStateProps = {
	ownedWorkspaceCount: number | undefined;
};

const MuxEmptyState: FC<MuxEmptyStateProps> = ({ ownedWorkspaceCount }) => {
	if (ownedWorkspaceCount === 0) {
		return (
			<EmptyState
				isCompact
				message="No workspaces yet"
				description="Create a workspace with the Mux app configured, then return here to open it."
			/>
		);
	}

	return (
		<EmptyState
			isCompact
			message="No Mux workspaces found"
			description="Your workspaces do not currently expose a Mux app. Add the Mux app to a workspace template and start the workspace."
		/>
	);
};

const MissingSelectedApp: FC = () => {
	return (
		<div className="flex h-full items-center justify-center p-6">
			<Alert severity="error" prominent className="max-w-2xl">
				<AlertTitle>Mux app was not found</AlertTitle>
				<AlertDescription>
					The selected workspace no longer exposes a Mux app. Choose another
					workspace or refresh the page.
				</AlertDescription>
			</Alert>
		</div>
	);
};
