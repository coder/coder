import { ArchiveIcon, PlusIcon } from "lucide-react";
import { useRef } from "react";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { Chat, ChatAutomation } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { ExperimentalBadge } from "#/components/Badge/PresetBadges";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Loader } from "#/components/Loader/Loader";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { useRestoreFocusOnClose } from "#/hooks/useRestoreFocusOnClose";
import { mobileViewportMediaQuery } from "#/utils/mobile";
import { AutomationDeleteDialog } from "./components/Automations/AutomationDeleteDialog";
import {
	AutomationOwnerFilter,
	type AutomationOwnerView,
} from "./components/Automations/AutomationOwnerFilter";
import { AutomationRow } from "./components/Automations/AutomationRow";
import { ChatTitleLink } from "./components/Automations/AutomationStatus";
import { SectionHeader } from "./components/SectionHeader";

export type AutomationRunError = {
	automation: ChatAutomation;
	error: unknown;
};

type AutomationChatsDialogState = {
	automation: ChatAutomation;
	chats: readonly Chat[] | undefined;
	isLoading: boolean;
	error: unknown;
	hasNextPage: boolean;
	isFetchingNextPage: boolean;
	onLoadMore: () => void;
	onClose: () => void;
};

type AutomationDeleteDialogState = {
	automation: ChatAutomation;
	isDeleting: boolean;
	error: unknown;
	onConfirm: () => void;
	onClose: () => void;
};

type AgentAutomationsPageViewProps = {
	header?: React.ReactNode;
	currentUserId: string;
	/** True for viewers, such as organization admins, who can delete any automation. */
	canDeleteAnyAutomation?: boolean;
	organizationSelector?: React.ReactNode;
	/** Which owners the list shows. Mine when unset. */
	ownerView?: AutomationOwnerView;
	/** Shows the Mine and All switch, for viewers who can read every owner's automations. */
	onOwnerViewChange?: (view: AutomationOwnerView) => void;
	automations: readonly ChatAutomation[] | undefined;
	isLoading: boolean;
	error: unknown;
	updatingAutomationId?: string;
	runningAutomationId?: string;
	runError?: AutomationRunError;
	onDismissRunError: () => void;
	onToggleEnabled: (automation: ChatAutomation, enabled: boolean) => void;
	onRunNow: (automation: ChatAutomation) => void;
	onViewChats: (automation: ChatAutomation) => void;
	onCreateAutomation: () => void;
	onEditAutomation: (automation: ChatAutomation) => void;
	onDeleteAutomation: (automation: ChatAutomation) => void;
	chatsDialog?: AutomationChatsDialogState;
	deleteDialog?: AutomationDeleteDialogState;
};

type AutomationChatsDialogProps = {
	state: AutomationChatsDialogState;
};

const AutomationChatsDialog: React.FC<AutomationChatsDialogProps> = ({
	state,
}) => {
	const restoreFocus = useRestoreFocusOnClose();
	let body: React.ReactNode;
	if (state.isLoading) {
		body = <Loader />;
	} else if (state.error && !state.chats) {
		body = <ErrorAlert error={state.error} />;
	} else if (!state.chats || state.chats.length === 0) {
		body = (
			<p className="m-0 text-sm text-content-secondary">
				No chats that you can open were created by or received messages from
				this automation.
			</p>
		);
	} else {
		body = (
			<ul className="m-0 flex list-none flex-col gap-2 p-0">
				{state.chats.map((chat) => (
					<li key={chat.id} className="flex items-center gap-2">
						<ChatTitleLink chatId={chat.id} title={chat.title} />
						{chat.archived && (
							<Badge size="sm">
								<ArchiveIcon />
								Archived
							</Badge>
						)}
					</li>
				))}
				{Boolean(state.error) && (
					<li>
						<ErrorAlert error={state.error} />
					</li>
				)}
				{state.hasNextPage && (
					<li>
						<Button
							size="sm"
							variant="outline"
							disabled={state.isFetchingNextPage}
							onClick={state.onLoadMore}
						>
							<Spinner loading={state.isFetchingNextPage} />
							Load more
						</Button>
					</li>
				)}
			</ul>
		);
	}
	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open) {
					state.onClose();
				}
			}}
		>
			<DialogContent onCloseAutoFocus={restoreFocus}>
				<DialogHeader>
					<DialogTitle>Chats for {state.automation.name}</DialogTitle>
					<DialogDescription>
						Chats this automation created or sent messages to.
					</DialogDescription>
				</DialogHeader>
				{/* Keeps Close in view when many chats are loaded. */}
				<div className="max-h-[60vh] overflow-y-auto">{body}</div>
				<DialogFooter>
					<Button variant="outline" onClick={state.onClose}>
						Close
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

export const AgentAutomationsPageView: React.FC<
	AgentAutomationsPageViewProps
> = ({
	header,
	currentUserId,
	canDeleteAnyAutomation = false,
	organizationSelector,
	ownerView = "mine",
	onOwnerViewChange,
	automations,
	isLoading,
	error,
	updatingAutomationId,
	runningAutomationId,
	runError,
	onDismissRunError,
	onToggleEnabled,
	onRunNow,
	onViewChats,
	onCreateAutomation,
	onEditAutomation,
	onDeleteAutomation,
	chatsDialog,
	deleteDialog,
}) => {
	const isMobile = useMediaQuery(mobileViewportMediaQuery);
	const newAutomationRef = useRef<HTMLButtonElement>(null);
	const showOwnerColumn = ownerView === "all" && !isMobile;
	let rows: React.ReactNode;
	if (isLoading) {
		rows = <TableLoader />;
	} else if (!automations || automations.length === 0) {
		rows = (
			<TableEmpty
				message={
					ownerView === "all"
						? "No automations in this organization"
						: "You have no automations yet"
				}
				description="Create a schedule or webhook that sends a prompt to an agent."
			/>
		);
	} else {
		rows = automations.map((automation) => (
			<AutomationRow
				key={automation.id}
				automation={automation}
				isOwner={automation.owner_id === currentUserId}
				canDelete={
					automation.owner_id === currentUserId || canDeleteAnyAutomation
				}
				compact={isMobile}
				showOwner={showOwnerColumn}
				isUpdating={updatingAutomationId === automation.id}
				isRunning={runningAutomationId === automation.id}
				isAnyRunPending={runningAutomationId !== undefined}
				onToggleEnabled={onToggleEnabled}
				onRunNow={onRunNow}
				onViewChats={onViewChats}
				onEdit={onEditAutomation}
				onDelete={onDeleteAutomation}
			/>
		));
	}

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
			{header}
			<div className="p-4 pt-8">
				<div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
					<SectionHeader
						label="Automations"
						badge={<ExperimentalBadge />}
						action={
							<div className="flex items-center gap-2">
								{onOwnerViewChange && (
									<AutomationOwnerFilter
										value={ownerView}
										onChange={onOwnerViewChange}
									/>
								)}
								{organizationSelector}
								<Button
									ref={newAutomationRef}
									size="sm"
									onClick={onCreateAutomation}
								>
									<PlusIcon />
									Create automation
								</Button>
							</div>
						}
					/>
					{/* Below the header, not inside it, so on phones the text uses the
					    full width instead of a column beside the buttons. */}
					<p className="-mt-4 m-0 text-sm text-content-secondary">
						Run agents in the background on a schedule or when an event arrives.
						Runs happen on your Coder deployment, so your browser and devices
						can be closed.
					</p>
					{runError && (
						<Alert
							severity="error"
							prominent
							dismissible
							onDismiss={onDismissRunError}
						>
							<AlertTitle>Could not run {runError.automation.name}</AlertTitle>
							<AlertDescription>
								{getErrorMessage(runError.error, "The run did not start.")}
								{getErrorDetail(runError.error) && (
									<span className="block">
										{getErrorDetail(runError.error)}
									</span>
								)}
							</AlertDescription>
						</Alert>
					)}
					{Boolean(error) &&
						(automations && automations.length > 0 ? (
							<Alert severity="warning" prominent>
								<AlertTitle>Could not refresh automations</AlertTitle>
								<AlertDescription>
									The automations shown may be out of date.{" "}
									{getErrorMessage(error, "The list did not refresh.")}
								</AlertDescription>
							</Alert>
						) : (
							<ErrorAlert error={error} />
						))}
					{(!error || automations) && (
						<Table aria-label="Automations">
							<TableHeader>
								<TableRow>
									<TableHead>Name</TableHead>
									{showOwnerColumn && <TableHead>Owner</TableHead>}
									{!isMobile && (
										<>
											<TableHead>Trigger</TableHead>
											<TableHead>Runs in</TableHead>
											<TableHead>Next run</TableHead>
										</>
									)}
									<TableHead>Enabled</TableHead>
									<TableHead className="text-right">Actions</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>{rows}</TableBody>
						</Table>
					)}
				</div>
			</div>
			{chatsDialog && <AutomationChatsDialog state={chatsDialog} />}
			{deleteDialog && (
				<AutomationDeleteDialog
					{...deleteDialog}
					isOwner={deleteDialog.automation.owner_id === currentUserId}
					fallbackFocusRef={newAutomationRef}
				/>
			)}
		</ScrollArea>
	);
};
