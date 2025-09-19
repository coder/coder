import { Label } from "@radix-ui/react-label";
import { Slot } from "@radix-ui/react-slot";
import { templateVersion } from "api/queries/templates";
import type { Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from "components/Dialog/Dialog";
import { Spinner } from "components/Spinner/Spinner";
import { TriangleAlert } from "lucide-react";
import { ACTIVE_BUILD_STATUSES } from "modules/workspaces/status";
import {
	type FC,
	type ForwardedRef,
	type ReactNode,
	useId,
	useRef,
	useState,
} from "react";
import { useQueries } from "react-query";
import { cn } from "utils/cn";

export const BatchUpdateModalForm: FC<BatchUpdateModalFormProps> = ({
	open,
	isProcessing,
	workspacesToUpdate,
	onCancel,
	onSubmit,
}) => {
	return (
		<Dialog
			open={open}
			onOpenChange={(newIsOpen) => {
				if (!newIsOpen) {
					onCancel();
				}
			}}
		>
			<DialogContent className="max-w-screen-md">
				<ReviewForm
					workspacesToUpdate={workspacesToUpdate}
					isProcessing={isProcessing}
					onCancel={onCancel}
					onSubmit={onSubmit}
				/>
			</DialogContent>
		</Dialog>
	);
};

type WorkspacePartitionByUpdateType = Readonly<{
	dormant: readonly Workspace[];
	noUpdateNeeded: readonly Workspace[];
	readyToUpdate: readonly Workspace[];
}>;

function separateWorkspacesByUpdateType(
	workspaces: readonly Workspace[],
): WorkspacePartitionByUpdateType {
	const noUpdateNeeded: Workspace[] = [];
	const dormant: Workspace[] = [];
	const readyToUpdate: Workspace[] = [];

	for (const ws of workspaces) {
		if (!ws.outdated) {
			noUpdateNeeded.push(ws);
			continue;
		}
		if (ws.dormant_at !== null) {
			dormant.push(ws);
			continue;
		}
		readyToUpdate.push(ws);
	}

	return { dormant, noUpdateNeeded, readyToUpdate };
}

type ReviewPanelProps = Readonly<{
	workspaceName: string;
	workspaceIconUrl: string;
	running: boolean;
	transitioning: boolean;
	label?: ReactNode;
	adornment?: ReactNode;
	className?: string;
}>;

const ReviewPanel: FC<ReviewPanelProps> = ({
	workspaceName,
	label,
	running,
	transitioning,
	workspaceIconUrl,
	className,
}) => {
	// Preemptively adding border to this component to help decouple the styling
	// from the rest of the components in this file, and make the core parts of
	// this component easier to reason about
	return (
		<div
			className={cn(
				"rounded-md px-4 py-3 border border-solid border-border text-sm",
				className,
			)}
		>
			<div className="flex flex-row flex-wrap grow items-center gap-3">
				<Avatar size="sm" variant="icon" src={workspaceIconUrl} />
				<div className="flex flex-col gap-0.5">
					<span className="flex flex-row items-center gap-2">
						<span className="leading-tight">{workspaceName}</span>
						{running && (
							<Badge size="xs" variant="warning" border="none">
								Running
							</Badge>
						)}
						{transitioning && (
							<Badge size="xs" variant="warning" border="none">
								Getting latest status
							</Badge>
						)}
					</span>
					<span className="text-xs leading-tight text-content-secondary">
						{label}
					</span>
				</div>
			</div>
		</div>
	);
};

const PanelListItem: FC<{ children: ReactNode }> = ({ children }) => {
	return (
		<li className="[&:not(:last-child)]:border-b-border [&:not(:last-child)]:border-b [&:not(:last-child)]:border-solid border-0">
			{children}
		</li>
	);
};

type TemplateNameChangeProps = Readonly<{
	oldTemplateVersionName: string;
	newTemplateVersionName: string;
}>;

const TemplateNameChange: FC<TemplateNameChangeProps> = ({
	oldTemplateVersionName: oldTemplateName,
	newTemplateVersionName: newTemplateName,
}) => {
	return (
		<>
			<span aria-hidden className="line-clamp-1">
				{oldTemplateName} &rarr; {newTemplateName}
			</span>
			<span className="sr-only">
				Workspace will go from version {oldTemplateName} to version{" "}
				{newTemplateName}
			</span>
		</>
	);
};

type RunningWorkspacesWarningProps = Readonly<{
	acceptedRisks: boolean;
	onAcceptedRisksChange: (newValue: boolean) => void;
	checkboxRef: ForwardedRef<HTMLButtonElement>;
	containerRef: ForwardedRef<HTMLDivElement>;
}>;

const RunningWorkspacesWarning: FC<RunningWorkspacesWarningProps> = ({
	acceptedRisks,
	onAcceptedRisksChange,
	checkboxRef,
	containerRef,
}) => {
	return (
		<div
			ref={containerRef}
			className="rounded-md border-border-warning border border-solid p-4"
		>
			<h4 className="m-0 font-semibold flex flex-row items-center gap-2 text-content-primary">
				<TriangleAlert className="text-content-warning" size={16} />
				Running workspaces detected
			</h4>

			<ul className="flex flex-col gap-1 m-0 px-5 pt-1.5 [&>li]:leading-snug text-content-secondary">
				<li>
					Updating a workspace will start it on its latest template version.
					This can delete non-persistent data.
				</li>
				<li>
					Anyone connected to a running workspace will be disconnected until the
					update is complete.
				</li>
				<li>Any unsaved data will be lost.</li>
			</ul>

			<Label className="flex flex-row gap-3 items-center leading-tight pt-6">
				<Checkbox
					ref={checkboxRef}
					checked={acceptedRisks}
					onCheckedChange={onAcceptedRisksChange}
				/>
				I acknowledge these risks.
			</Label>
		</div>
	);
};

type ContainerProps = Readonly<{
	asChild?: boolean;
	children?: ReactNode;
}>;

const Container: FC<ContainerProps> = ({ children, asChild = false }) => {
	const Wrapper = asChild ? Slot : "div";
	return (
		<Wrapper className="max-h-[80vh] flex flex-col flex-nowrap">
			{children}
		</Wrapper>
	);
};

type ContainerBodyProps = Readonly<{
	headerText: ReactNode;
	description: ReactNode;
	showDescription?: boolean;
	children?: ReactNode;
}>;

const ContainerBody: FC<ContainerBodyProps> = ({
	children,
	headerText,
	description,
	showDescription = false,
}) => {
	return (
		// Have to subtract parent padding via margin values and then add it
		// back as child padding so that there's no risk of the scrollbar
		// covering up content when the container gets tall enough to overflow
		<div className="overflow-y-auto flex flex-col gap-3 -mx-8 -mt-8 p-8">
			<div className="flex flex-col gap-3">
				<DialogTitle asChild>
					<h3 className="text-3xl font-semibold m-0 leading-tight">
						{headerText}
					</h3>
				</DialogTitle>

				<DialogDescription
					className={cn("m-0 text-base", !showDescription && "sr-only")}
				>
					{description}
				</DialogDescription>
			</div>

			{children}
		</div>
	);
};

type ContainerFooterProps = Readonly<{
	className?: string;
	children?: ReactNode;
}>;

const ContainerFooter: FC<ContainerFooterProps> = ({ children, className }) => {
	return (
		<div
			className={cn(
				// Also have to subtract padding here to make sure footer is
				// full-bleed, and there's no risk of the border getting
				// confused for the outline of one of the panels
				"border-0 border-t border-solid border-t-border pt-8 -mx-8 px-8",
				className,
			)}
		>
			{children}
		</div>
	);
};

type WorkspacesListSectionProps = Readonly<{
	headerText: ReactNode;
	description: ReactNode;
	children?: ReactNode;
}>;

const WorkspacesListSection: FC<WorkspacesListSectionProps> = ({
	children,
	headerText,
	description,
}) => {
	return (
		<section className="flex flex-col gap-3.5">
			<div className="max-w-prose">
				<h4 className="m-0">{headerText}</h4>
				<p className="m-0 text-sm leading-snug text-content-secondary">
					{description}
				</p>
			</div>

			<ul className="m-0 list-none p-0 flex flex-col rounded-md border border-solid border-border">
				{children}
			</ul>
		</section>
	);
};

// Used to force the user to acknowledge that batch updating has risks in
// certain situations and could destroy their data
type RisksStage = "notAccepted" | "accepted" | "failedValidation";

type ReviewFormProps = Readonly<{
	workspacesToUpdate: readonly Workspace[];
	isProcessing: boolean;
	onCancel: () => void;
	onSubmit: () => void;
}>;

const ReviewForm: FC<ReviewFormProps> = ({
	workspacesToUpdate,
	isProcessing,
	onCancel,
	onSubmit,
}) => {
	const hookId = useId();
	const [stage, setStage] = useState<RisksStage>("notAccepted");
	const risksContainerRef = useRef<HTMLDivElement>(null);
	const risksCheckboxRef = useRef<HTMLButtonElement>(null);

	// Dormant workspaces can't be activated without activating them first. For
	// now, we'll only show the user that some workspaces can't be updated, and
	// then skip over them for all other update logic
	const { dormant, noUpdateNeeded, readyToUpdate } =
		separateWorkspacesByUpdateType(workspacesToUpdate);

	// The workspaces don't have all necessary data by themselves, so we need to
	// fetch the unique template versions, and massage the results
	const uniqueTemplateVersionIds = new Set<string>(
		readyToUpdate.map((ws) => ws.template_active_version_id),
	);
	const templateVersionQueries = useQueries({
		queries: [...uniqueTemplateVersionIds].map((id) => templateVersion(id)),
	});

	// React Query persists previous errors even if a query is no longer in the
	// error state, so we need to explicitly check the isError property to see
	// if any of the queries actively have an error
	const error = templateVersionQueries.find((q) => q.isError)?.error;

	const hasWorkspaces = workspacesToUpdate.length > 0;
	const someWorkspacesCanBeUpdated = readyToUpdate.length > 0;

	const formIsNeeded = someWorkspacesCanBeUpdated || dormant.length > 0;
	if (!formIsNeeded) {
		return (
			<Container>
				<ContainerBody
					headerText={
						hasWorkspaces
							? "All workspaces up to date"
							: "No workspaces selected"
					}
					showDescription
					description={
						hasWorkspaces ? (
							<>
								None of the{" "}
								<span className="text-content-primary font-semibold">
									{workspacesToUpdate.length}
								</span>{" "}
								selected workspaces need updates.
							</>
						) : (
							"Nothing to update."
						)
					}
				>
					{error !== undefined && <ErrorAlert error={error} />}
				</ContainerBody>

				<ContainerFooter className="flex flex-row justify-end">
					<Button variant="outline" onClick={onCancel}>
						Close
					</Button>
				</ContainerFooter>
			</Container>
		);
	}

	const runningIds = new Set<string>(
		readyToUpdate
			.filter((ws) => ws.latest_build.status === "running")
			.map((ws) => ws.id),
	);

	/**
	 * Two things:
	 * 1. We have to make sure that we don't let the user submit anything while
	 *    workspaces are transitioning, or else we'll run into a race condition.
	 *    If a user starts a workspace, and then immediately batch-updates it,
	 *    the workspace won't be in the running state yet. We need to issue
	 *    warnings about how updating running workspaces is a destructive
	 *    action, but if the the user goes through the form quickly enough,
	 *    they'll be able to update without seeing the warning.
	 * 2. Just to be on the safe side, we also need to derive the transitioning
	 *    IDs from all checked workspaces, because the separation result could
	 *    theoretically change on re-render after any workspace state
	 *    transitions end.
	 */
	const transitioningIds = new Set<string>(
		workspacesToUpdate
			.filter((ws) => ACTIVE_BUILD_STATUSES.includes(ws.latest_build.status))
			.map((ws) => ws.id),
	);

	const hasRunningWorkspaces = runningIds.size > 0;
	const risksAcknowledged = !hasRunningWorkspaces || stage === "accepted";
	const failedValidationId =
		stage === "failedValidation" ? `${hookId}-failed-validation` : undefined;

	// For UX/accessibility reasons, we're splitting a lot of hairs between
	// various invalid/disabled states. We do not just want to throw a blanket
	// `disabled` attribute on a button and call it a day. The most important
	// thing is that we need to give the user feedback on how to get unstuck if
	// they fail any input validations
	const safeToSubmit = transitioningIds.size === 0 && error === undefined;
	const buttonIsDisabled = !safeToSubmit || isProcessing;
	const submitIsValid =
		risksAcknowledged && error === undefined && readyToUpdate.length > 0;

	return (
		<Container asChild>
			<form
				onSubmit={(e) => {
					e.preventDefault();
					if (!someWorkspacesCanBeUpdated) {
						onCancel();
						return;
					}
					if (submitIsValid) {
						onSubmit();
						return;
					}
					if (stage === "accepted") {
						return;
					}

					setStage("failedValidation");
					// Makes sure that if the modal is long enough to scroll and
					// if the warning section checkbox isn't on screen anymore,
					// the warning section goes back to being on screen
					risksContainerRef.current?.scrollIntoView({
						behavior: "smooth",
					});
					risksCheckboxRef.current?.focus();
				}}
			>
				<ContainerBody
					headerText="Review updates"
					description="The following workspaces will be updated:"
				>
					<div className="flex flex-col gap-4">
						{error !== undefined && <ErrorAlert error={error} />}

						{hasRunningWorkspaces && (
							<RunningWorkspacesWarning
								checkboxRef={risksCheckboxRef}
								containerRef={risksContainerRef}
								acceptedRisks={stage === "accepted"}
								onAcceptedRisksChange={(newChecked) => {
									if (newChecked) {
										setStage("accepted");
									} else {
										setStage("notAccepted");
									}
								}}
							/>
						)}

						{readyToUpdate.length > 0 && (
							<WorkspacesListSection
								headerText="Ready to update"
								description="These workspaces will have their templates be updated to the latest version."
							>
								{readyToUpdate.map((ws) => {
									const matchedQuery = templateVersionQueries.find(
										(q) => q.data?.id === ws.template_active_version_id,
									);
									const newTemplateName = matchedQuery?.data?.name;

									return (
										<PanelListItem key={ws.id}>
											<ReviewPanel
												className="border-none"
												running={runningIds.has(ws.id)}
												transitioning={transitioningIds.has(ws.id)}
												workspaceName={ws.name}
												workspaceIconUrl={ws.template_icon}
												label={
													newTemplateName !== undefined && (
														<TemplateNameChange
															newTemplateVersionName={newTemplateName}
															oldTemplateVersionName={
																ws.latest_build.template_version_name
															}
														/>
													)
												}
											/>
										</PanelListItem>
									);
								})}
							</WorkspacesListSection>
						)}

						{noUpdateNeeded.length > 0 && (
							<WorkspacesListSection
								headerText="Already updated"
								description="These workspaces are already updated and will be skipped."
							>
								{noUpdateNeeded.map((ws) => (
									<PanelListItem key={ws.id}>
										<ReviewPanel
											className="border-none"
											running={false}
											transitioning={transitioningIds.has(ws.id)}
											workspaceName={ws.name}
											workspaceIconUrl={ws.template_icon}
										/>
									</PanelListItem>
								))}
							</WorkspacesListSection>
						)}

						{dormant.length > 0 && (
							<WorkspacesListSection
								headerText="Dormant workspaces"
								description={
									<>
										Dormant workspaces cannot be updated without first
										activating the workspace. They will always be skipped during
										batch updates.
									</>
								}
							>
								{dormant.map((ws) => (
									<li
										key={ws.id}
										className="[&:not(:last-child)]:border-b-border [&:not(:last-child)]:border-b [&:not(:last-child)]:border-solid border-0"
									>
										<ReviewPanel
											className="border-none"
											running={false}
											transitioning={transitioningIds.has(ws.id)}
											workspaceName={ws.name}
											workspaceIconUrl={ws.template_icon}
										/>
									</li>
								))}
							</WorkspacesListSection>
						)}
					</div>
				</ContainerBody>

				<ContainerFooter>
					<div className="flex flex-row flex-wrap justify-end gap-4">
						<Button variant="outline" onClick={onCancel}>
							Cancel
						</Button>
						<Button
							variant="default"
							type="submit"
							disabled={buttonIsDisabled}
							aria-describedby={failedValidationId}
						>
							{isProcessing && (
								<>
									<Spinner loading />
									<span className="sr-only">
										Waiting for workspaces to finish processing
									</span>
								</>
							)}

							{!safeToSubmit && !isProcessing && (
								<span className="sr-only">
									Unable to complete batch update because of workspace error
								</span>
							)}

							{someWorkspacesCanBeUpdated ? (
								<span aria-hidden={buttonIsDisabled}>Update</span>
							) : (
								"Close"
							)}
						</Button>
					</div>

					{stage === "failedValidation" && (
						<p
							id={failedValidationId}
							className="m-0 text-highlight-red text-right text-sm pt-2"
						>
							Please acknowledge risks to continue.
						</p>
					)}
				</ContainerFooter>
			</form>
		</Container>
	);
};

type BatchUpdateModalFormProps = Readonly<{
	open: boolean;
	isProcessing: boolean;
	workspacesToUpdate: readonly Workspace[];
	onCancel: () => void;
	onSubmit: () => void;
}>;
