import { chats, createChat } from "api/queries/chats";
import type { Chat } from "api/typesGenerated";
import { Spinner } from "components/Spinner/Spinner";
import { MessageSquareIcon, SendIcon } from "lucide-react";
import {
	type FC,
	type KeyboardEvent,
	useEffect,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, useNavigate } from "react-router";
import { pageTitle } from "utils/page";

const ChatsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const chatsQuery = useQuery(chats());
	const createChatMutation = useMutation(createChat(queryClient));
	const [message, setMessage] = useState("");
	const inputRef = useRef<HTMLTextAreaElement>(null);

	// Auto-focus the input on mount
	useEffect(() => {
		inputRef.current?.focus();
	}, []);

	const handleSend = async () => {
		if (!message.trim() || createChatMutation.isPending) return;

		try {
			// Create a new chat with the message as the initial content
			const created = await createChatMutation.mutateAsync({
				title: "", // Let the backend/UI generate a title
				provider: "anthropic",
				model: "claude-sonnet-4-20250514",
			});

			// Navigate to the chat and let it send the message
			navigate(`/chats/${created.id}?message=${encodeURIComponent(message)}`);
		} catch (error) {
			console.error("Failed to create chat:", error);
		}
	};

	const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			handleSend();
		}
	};

	return (
		<>
			<title>{pageTitle("Chats")}</title>
			<div className="flex flex-col h-full max-w-4xl mx-auto px-4 py-8">
				{/* Chat input at top */}
				<div className="mb-8">
					<div className="relative">
						<textarea
							ref={inputRef}
							value={message}
							onChange={(e) => setMessage(e.target.value)}
							onKeyDown={handleKeyDown}
							placeholder="Send a message to start a new chat..."
							className="w-full min-h-[100px] p-4 pr-12 rounded-lg border border-border bg-surface-primary text-content-primary placeholder:text-content-secondary resize-none focus:outline-none focus:ring-2 focus:ring-border-active"
							disabled={createChatMutation.isPending}
						/>
						<button
							type="button"
							onClick={handleSend}
							disabled={!message.trim() || createChatMutation.isPending}
							className="absolute right-3 bottom-3 p-2 rounded-md bg-surface-invert-primary text-content-invert hover:bg-surface-invert-secondary disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{createChatMutation.isPending ? (
								<Spinner size="sm" loading />
							) : (
								<SendIcon className="size-5" />
							)}
						</button>
					</div>
				</div>

				{/* Recent chats section */}
				<div className="flex-1">
					<h2 className="text-lg font-semibold text-content-primary mb-4">
						Recent Chats
					</h2>

					{chatsQuery.isLoading && (
						<div className="flex items-center justify-center py-8">
							<Spinner loading />
						</div>
					)}

					{chatsQuery.isError && (
						<div className="text-content-destructive py-4">
							Failed to load chats. Please try again.
						</div>
					)}

					{chatsQuery.isSuccess && chatsQuery.data.length === 0 && (
						<div className="flex flex-col items-center justify-center py-12 text-content-secondary">
							<MessageSquareIcon className="size-10 mb-3" />
							<p className="text-sm">No chats yet. Send a message to start!</p>
						</div>
					)}

					{chatsQuery.isSuccess && chatsQuery.data.length > 0 && (
						<div className="space-y-2">
							{chatsQuery.data.map((chat: Chat) => (
								<Link
									key={chat.id}
									to={`/chats/${chat.id}`}
									className="block p-4 rounded-lg border border-border hover:bg-surface-secondary transition-colors"
								>
									<div className="flex items-center justify-between">
										<div className="flex items-center gap-3">
											<MessageSquareIcon className="size-5 text-content-secondary" />
											<span className="font-medium text-content-primary">
												{chat.title || "Untitled Chat"}
											</span>
										</div>
										<span className="text-sm text-content-secondary">
											{new Date(chat.created_at).toLocaleDateString()}
										</span>
									</div>
									<div className="mt-1 text-sm text-content-secondary ml-8">
										{chat.provider} / {chat.model}
									</div>
								</Link>
							))}
						</div>
					)}
				</div>
			</div>
		</>
	);
};

export default ChatsPage;
