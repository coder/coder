import type { FC } from "react";
import { useQuery, useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams, Outlet } from "react-router-dom";
import { chats, createChat } from "api/queries/chats";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { PlusIcon } from "lucide-react";
import { AgentsSidebar } from "./AgentsSidebar";
import { pageTitle } from "utils/page";

export const AgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();

	const chatsQuery = useQuery(chats());
	const createMutation = useMutation(createChat(queryClient));

	const handleCreateChat = async () => {
		const newChat = await createMutation.mutateAsync({
			title: "New Agent Chat",
		});
		navigate(`/agents/${newChat.id}`);
	};

	if (chatsQuery.isLoading) {
		return <Loader />;
	}

	const chatList = chatsQuery.data ?? [];

	return (
		<>
			<title>{pageTitle("Agents")}</title>
			<Margins>
				<PageHeader
					actions={
						<Button
							onClick={handleCreateChat}
							disabled={createMutation.isPending}
						>
							<PlusIcon />
							New Agent
						</Button>
					}
				>
					<PageHeaderTitle>Agents</PageHeaderTitle>
				</PageHeader>

				<div className="flex gap-6 items-start">
					<AgentsSidebar
						chats={chatList}
						selectedChatId={agentId}
						onSelect={(id) => navigate(`/agents/${id}`)}
					/>

					<div className="flex-1 min-w-0">
						{agentId ? (
							<Outlet />
						) : chatList.length === 0 ? (
							<EmptyState
								message="No agents yet"
								description="Create a new agent to start chatting with AI"
								cta={
									<Button
										onClick={handleCreateChat}
										disabled={createMutation.isPending}
									>
										<PlusIcon />
										Create Agent
									</Button>
								}
							/>
						) : (
							<EmptyState
								message="Select an agent"
								description="Choose an agent from the sidebar to view the conversation"
							/>
						)}
					</div>
				</div>
			</Margins>
		</>
	);
};

export default AgentsPage;
