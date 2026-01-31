import { type FC, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "react-query";
import { chat, createChatMessage } from "api/queries/chats";
import { Loader } from "components/Loader/Loader";
import { Button } from "components/Button/Button";
import { SendIcon } from "lucide-react";
import { cn } from "utils/cn";
import { FilesChangedPanel } from "./FilesChangedPanel";

export const AgentDetail: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	const queryClient = useQueryClient();
	const [input, setInput] = useState("");

	const chatQuery = useQuery({
		...chat(agentId!),
		enabled: !!agentId,
	});

	const sendMutation = useMutation(createChatMessage(queryClient, agentId!));

	const handleSend = async () => {
		if (!input.trim()) return;

		await sendMutation.mutateAsync({
			role: "user",
			content: input,
		});
		setInput("");
	};

	const handleKeyPress = (e: React.KeyboardEvent) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			handleSend();
		}
	};

	if (chatQuery.isLoading) {
		return <Loader />;
	}

	if (!chatQuery.data) {
		return (
			<div className="p-8 text-center text-content-secondary">
				Chat not found
			</div>
		);
	}

	const { chat: chatData, messages } = chatQuery.data;

	return (
		<div className="flex gap-4 h-[calc(100vh-200px)]">
			{/* Main chat area */}
			<div className="flex-1 border border-border-default rounded-lg bg-surface-primary flex flex-col min-w-0">
				{/* Chat header */}
				<div className="px-4 py-3 border-b border-border-default">
					<h2 className="text-lg font-medium m-0">{chatData.title}</h2>
					<span className="text-xs text-content-secondary">
						Status: {chatData.status}
					</span>
				</div>

				{/* Messages area */}
				<div className="flex-1 overflow-y-auto p-4 space-y-4">
					{messages
						.filter((m) => !m.hidden)
						.map((message) => (
							<div
								key={message.id}
								className={cn(
									"p-3 rounded-lg max-w-[80%]",
									message.role === "user"
										? "bg-surface-secondary ml-auto"
										: "bg-surface-tertiary",
								)}
							>
								<div className="text-xs text-content-secondary mb-1">
									{message.role}
								</div>
								<div className="text-sm">
									{typeof message.content === "string"
										? message.content
										: JSON.stringify(message.content)}
								</div>
							</div>
						))}
				</div>

				{/* Input area */}
				<div className="p-4 border-t border-border-default">
					<div className="flex gap-2">
						<input
							type="text"
							className="flex-1 px-3 py-2 border border-border-default rounded-md bg-surface-primary text-sm focus:outline-none focus:ring-2 focus:ring-content-link"
							placeholder="Type a message..."
							value={input}
							onChange={(e) => setInput(e.target.value)}
							onKeyPress={handleKeyPress}
							disabled={sendMutation.isPending}
						/>
						<Button
							onClick={handleSend}
							disabled={sendMutation.isPending || !input.trim()}
							size="icon"
						>
							<SendIcon />
						</Button>
					</div>
				</div>
			</div>

			{/* Files changed sidebar */}
			<div className="w-64 flex-shrink-0 border border-border-default rounded-lg bg-surface-primary">
				<FilesChangedPanel chatId={agentId!} />
			</div>
		</div>
	);
};

export default AgentDetail;
