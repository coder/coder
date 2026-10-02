import { ArchiveIcon } from "lucide-react";
import { useState } from "react";
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
import { mobileViewportMediaQuery } from "#/utils/mobile";
import {
	AutomationRow,
	ChatTitleLink,
} from "./components/Automations/AutomationRow";
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

type AgentAutomationsPageViewProps = {
	header?: React.ReactNode;
	currentUserId: string;
	organizationSelector?: React.ReactNode;
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
	chatsDialog?: AutomationChatsDialogState;
	editorDialog?: React.ReactNode;
	webhookSecretDialog?: React.ReactNode;
};

type AutomationChatsDialogProps = {
	state: AutomationChatsDialogState;
};

const AutomationChatsDialog: React.FC<AutomationChatsDialogProps> = ({
	state,
}) => {
	// The dialog opens from a row button rather than a DialogTrigger, so Radix
	// has no trigger to return focus to on close. It mounts right after the
	// View chats button is activated, so the focused element is the opener.
	const [opener] = useState(() => document.activeElement);
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
			<DialogContent
				onCloseAutoFocus={(event) => {
					if (opener instanceof HTMLElement && opener.isConnected) {
						event.preventDefault();
						opener.focus();
					}
				}}
			>
				<DialogHeader>
					<DialogTitle>Chats for {state.automation.name}</DialogTitle>
					<DialogDescription>
						Chats this automation created or sent messages to.
					</DialogDescription>
				</DialogHeader>
				{body}
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
	organizationSelector,
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
	chatsDialog,
	editorDialog,
	webhookSecretDialog,
}) => {
	const isMobile = useMediaQuery(mobileViewportMediaQuery);
	let rows: React.ReactNode;
	if (isLoading) {
		rows = <TableLoader />;
	} else if (!automations || automations.length === 0) {
		rows = (
			<TableEmpty
				message="No automations yet"
				description="Create a schedule or webhook that sends a prompt to an agent."
			/>
		);
	} else {
		rows = automations.map((automation) => (
			<AutomationRow
				key={automation.id}
				automation={automation}
				isOwner={automation.owner_id === currentUserId}
				compact={isMobile}
				isUpdating={updatingAutomationId === automation.id}
				isRunning={runningAutomationId === automation.id}
				isAnyRunPending={runningAutomationId !== undefined}
				onToggleEnabled={onToggleEnabled}
				onRunNow={onRunNow}
				onViewChats={onViewChats}
				onEdit={onEditAutomation}
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
						description="Run agents in the background on a schedule or when an event arrives. Runs happen on your Coder deployment, so your browser and devices can be closed."
						action={
							<div className="flex items-center gap-2">
								{organizationSelector}
								<Button size="sm" onClick={onCreateAutomation}>
									New automation
								</Button>
							</div>
						}
					/>
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
									{!isMobile && (
										<>
											<TableHead>Trigger</TableHead>
											<TableHead>Target</TableHead>
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
			{editorDialog}
			{webhookSecretDialog}
		</ScrollArea>
	);
};
