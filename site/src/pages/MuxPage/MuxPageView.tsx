import type { FC } from "react";
import type { Workspace } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
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
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
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
	onSelectWorkspace: (workspaceId: string | undefined) => void;
};

export const MuxPageView: FC<MuxPageViewProps> = ({
	isLoading,
	error,
	ownedWorkspaceCount,
	muxWorkspaces,
	selectedWorkspace,
	selectedMuxCandidate,
	onSelectWorkspace,
}) => {
	const selectedOption = selectedWorkspace
		? {
				label: `${selectedWorkspace.owner_name}/${selectedWorkspace.name}`,
				value: selectedWorkspace.id,
			}
		: undefined;
	const resolvedPageTitle = selectedWorkspace
		? pageTitle(`Mux - ${selectedWorkspace.name}`)
		: pageTitle("Mux");

	return (
		<DashboardFullPage>
			<title>{resolvedPageTitle}</title>
			<Margins className="flex min-h-0 flex-1 flex-col pb-6">
				<PageHeader className="shrink-0">
					<PageHeaderTitle>Mux</PageHeaderTitle>
					<PageHeaderSubtitle>
						Open the Mux app from one of your workspaces.
					</PageHeaderSubtitle>
				</PageHeader>

				{isLoading ? (
					<Loader label="Loading workspaces" />
				) : error ? (
					<ErrorAlert error={error} />
				) : muxWorkspaces.length === 0 ? (
					<MuxEmptyState ownedWorkspaceCount={ownedWorkspaceCount} />
				) : (
					<>
						<div className="mb-4 flex shrink-0 flex-col gap-2 sm:max-w-md">
							<span
								id="mux-workspace-select-label"
								className="text-sm font-medium text-content-primary"
							>
								Select workspace
							</span>
							<Combobox
								value={selectedWorkspace?.id}
								onValueChange={onSelectWorkspace}
							>
								<ComboboxTrigger asChild>
									<ComboboxButton
										aria-labelledby="mux-workspace-select-label"
										placeholder="Choose a workspace"
										selectedOption={selectedOption}
										width={384}
									/>
								</ComboboxTrigger>
								<ComboboxContent align="start" className="w-96 max-w-[90vw]">
									<ComboboxInput placeholder="Search workspaces" />
									<ComboboxList>
										<ComboboxEmpty>No workspaces found.</ComboboxEmpty>
										{muxWorkspaces.map((workspace) => (
											<WorkspaceOption
												key={workspace.id}
												workspace={workspace}
											/>
										))}
									</ComboboxList>
								</ComboboxContent>
							</Combobox>
						</div>

						<section
							aria-label="Mux app"
							className="min-h-0 flex-1 overflow-hidden rounded-lg border border-solid border-border"
						>
							{selectedWorkspace && selectedMuxCandidate ? (
								<MuxIframe
									workspace={selectedWorkspace}
									candidate={selectedMuxCandidate}
								/>
							) : selectedWorkspace ? (
								<MissingSelectedApp />
							) : (
								<EmptyState
									isCompact
									className="h-full"
									message="Choose a workspace"
									description="Select a workspace above to open its Mux app."
								/>
							)}
						</section>
					</>
				)}
			</Margins>
		</DashboardFullPage>
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
				message="No workspaces yet"
				description="Create a workspace with the Mux app configured, then return here to open it."
			/>
		);
	}

	return (
		<EmptyState
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
