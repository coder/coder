import { useRef, useState } from "react";
import { useMutation, useQueries, useQueryClient } from "react-query";
import { Navigate } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	chatAutomations,
	deleteChatAutomation,
} from "#/api/queries/chatAutomations";
import type { ChatAutomation } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { toast } from "#/components/Toaster/toast";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentPageHeader } from "../components/AgentPageHeader";
import { useDialogFocusReturn } from "../components/useDialogFocusReturn";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";

const AgentAutomationsPage: React.FC = () => {
	const { organizations, experiments } = useDashboard();
	const enabled = experiments.includes("chat-automations");
	const automationQueries = useQueries({
		queries: organizations.map((organization) => ({
			...chatAutomations(organization.id),
			enabled,
		})),
	});
	const queryClient = useQueryClient();
	const deleteMutation = useMutation(deleteChatAutomation(queryClient));
	const { rememberTrigger, restoreFocus } = useDialogFocusReturn();
	const focusFallbackRef = useRef<HTMLElement>(null);
	const [deleteDialog, setDeleteDialog] = useState<{
		automation: ChatAutomation | null;
		open: boolean;
		deleted: boolean;
	}>({ automation: null, open: false, deleted: false });

	if (!enabled) {
		return <Navigate to="/agents" replace />;
	}

	const automations = automationQueries.flatMap((query) => query.data ?? []);
	const loadErrors = organizations.flatMap((organization, index) => {
		const query = automationQueries[index];
		return query.isError
			? [{ organization, error: query.error, isRetrying: query.isFetching }]
			: [];
	});

	const handleDelete = () => {
		const automation = deleteDialog.automation;
		if (!automation) {
			return;
		}
		deleteMutation.mutate(automation, {
			onSuccess: () => {
				// A dialog opened for another automation must survive this request.
				setDeleteDialog((current) =>
					current.automation?.id === automation.id
						? { ...current, open: false, deleted: true }
						: current,
				);
			},
			onError: (error) => {
				toast.error(getErrorMessage(error, "Failed to delete automation."));
			},
		});
	};

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
			<AgentPageHeader />
			<div className="p-4 pt-8">
				<section
					ref={focusFallbackRef}
					tabIndex={-1}
					aria-label="Automations"
					className="mx-auto w-full max-w-3xl"
				>
					<AgentAutomationsPageView
						automations={automations}
						organizations={organizations}
						isLoading={automationQueries.some((query) => query.isLoading)}
						loadErrors={loadErrors}
						onRetry={(organizationId) => {
							const index = organizations.findIndex(
								(organization) => organization.id === organizationId,
							);
							void automationQueries[index]?.refetch();
						}}
						onMenuTrigger={rememberTrigger}
						onDeleteAutomation={(automation) =>
							setDeleteDialog({ automation, open: true, deleted: false })
						}
					/>
					<DeleteDialog
						isOpen={deleteDialog.open}
						onConfirm={handleDelete}
						onCancel={() =>
							setDeleteDialog((current) => ({ ...current, open: false }))
						}
						onCloseAutoFocus={(event) => {
							if (!deleteDialog.deleted) {
								restoreFocus(event);
							}
							if (!event.defaultPrevented) {
								event.preventDefault();
								focusFallbackRef.current?.focus();
							}
						}}
						entity="automation"
						name={deleteDialog.automation?.name ?? ""}
						confirmLoading={deleteMutation.isPending}
						info="Existing chats will be kept."
					/>
				</section>
			</div>
		</ScrollArea>
	);
};

export default AgentAutomationsPage;
