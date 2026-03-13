import {
	chatMCPServerConfigs,
	createChatMCPServerConfig,
	deleteChatMCPServerConfig,
	updateChatMCPServerConfig,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { type FC, type ReactNode, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { SectionHeader } from "../SectionHeader";
import { MCPServerForm } from "./MCPServerForm";
import { MCPServerList } from "./MCPServerList";

interface MCPServerAdminPanelProps {
	sectionLabel?: string;
	sectionDescription?: string;
	sectionBadge?: ReactNode;
}

type MCPServerView =
	| { mode: "list" }
	| { mode: "add" }
	| { mode: "edit"; server: TypesGen.ChatMCPServerConfig };

export const MCPServerAdminPanel: FC<MCPServerAdminPanelProps> = ({
	sectionLabel,
	sectionDescription,
	sectionBadge,
}) => {
	const queryClient = useQueryClient();
	const serversQuery = useQuery(chatMCPServerConfigs());
	const createMutation = useMutation(createChatMCPServerConfig(queryClient));
	const updateMutation = useMutation(updateChatMCPServerConfig(queryClient));
	const deleteMutation = useMutation(deleteChatMCPServerConfig(queryClient));
	const [view, setView] = useState<MCPServerView>({ mode: "list" });

	const servers = serversQuery.data ?? [];
	const isMutating =
		createMutation.isPending ||
		updateMutation.isPending ||
		deleteMutation.isPending;

	if (view.mode === "edit") {
		const current = servers.find((s) => s.id === view.server.id);
		if (!current) {
			setView({ mode: "list" });
			return null;
		}
		return (
			<MCPServerForm
				server={current}
				isMutating={isMutating}
				onSave={async (req) => {
					await updateMutation.mutateAsync({ id: current.id, req });
					setView({ mode: "list" });
				}}
				onDelete={async () => {
					await deleteMutation.mutateAsync(current.id);
					setView({ mode: "list" });
				}}
				onBack={() => setView({ mode: "list" })}
			/>
		);
	}

	if (view.mode === "add") {
		return (
			<MCPServerForm
				isMutating={isMutating}
				onSave={async (req) => {
					await createMutation.mutateAsync(
						req as TypesGen.CreateChatMCPServerRequest,
					);
					setView({ mode: "list" });
				}}
				onBack={() => setView({ mode: "list" })}
			/>
		);
	}

	return (
		<>
			{sectionLabel && (
				<SectionHeader
					label={sectionLabel}
					description={
						sectionDescription ??
						"Configure MCP servers to extend agent capabilities with external tools."
					}
					badge={sectionBadge}
				/>
			)}
			<MCPServerList
				servers={servers}
				isLoading={serversQuery.isLoading}
				onAdd={() => setView({ mode: "add" })}
				onEdit={(server) => setView({ mode: "edit", server })}
			/>
			{(serversQuery.isError ||
				createMutation.isError ||
				updateMutation.isError ||
				deleteMutation.isError) && (
				<p className="mt-3 text-xs text-content-destructive">
					{(
						serversQuery.error ||
						createMutation.error ||
						updateMutation.error ||
						deleteMutation.error
					)?.message ?? "An error occurred."}
				</p>
			)}
		</>
	);
};
