import { chats, createChat } from "api/queries/chats";
import type { Chat } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Spinner } from "components/Spinner/Spinner";
import { Textarea } from "components/Textarea/Textarea";
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
				model: "claude-opus-4-5",
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
			<Margins>
				<PageHeader>
					<PageHeaderTitle>Chats</PageHeaderTitle>
					<PageHeaderSubtitle>
						Chat with AI assistants that can execute tools.
					</PageHeaderSubtitle>
				</PageHeader>
				<main className="space-y-6 pb-8">
					<section className="rounded-xl border border-border bg-surface-primary shadow-sm p-4">
						<Textarea
							ref={inputRef}
							value={message}
							onChange={(e) => setMessage(e.target.value)}
							onKeyDown={handleKeyDown}
							placeholder="Send a message to start a new chat..."
							className="min-h-[120px] resize-none bg-surface-primary"
							disabled={createChatMutation.isPending}
						/>
						<div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
							<p className="text-xs text-content-secondary">
								Press Enter to send. Shift+Enter for a new line.
							</p>
							<Button
								onClick={handleSend}
								disabled={!message.trim() || createChatMutation.isPending}
								className="self-start sm:self-auto"
							>
								<Spinner loading={createChatMutation.isPending}>
									<SendIcon className="size-4" />
								</Spinner>
								Send
							</Button>
						</div>
					</section>
					<section className="rounded-xl border border-border bg-surface-primary shadow-sm">
						<div className="flex items-center justify-between border-b border-border px-4 py-3">
							<h2 className="text-sm font-semibold text-content-primary">Recent Chats</h2>
						</div>
						{chatsQuery.isLoading && (
							<div className="flex items-center justify-center py-8">
								<Spinner loading />
							</div>
						)}

						{chatsQuery.isError && (
							<div className="px-4 py-4 text-content-destructive">
								Failed to load chats. Please try again.
							</div>
						)}

						{chatsQuery.isSuccess && chatsQuery.data.length === 0 && (
							<div className="flex flex-col items-center justify-center py-12 text-content-secondary">
								<MessageSquareIcon className="size-9 mb-2" />
								<p className="text-sm">No chats yet. Send a message to start!</p>
							</div>
						)}

						{chatsQuery.isSuccess && chatsQuery.data.length > 0 && (
							<div className="divide-y divide-border">
								{chatsQuery.data.map((chat: Chat) => (
									<Link
										key={chat.id}
										to={`/chats/${chat.id}`}
										className="flex items-center justify-between px-4 py-3 transition-colors hover:bg-surface-secondary"
									>
										<div className="flex items-center gap-3">
											<div className="flex size-9 items-center justify-center rounded-full bg-surface-secondary text-content-secondary">
												<MessageSquareIcon className="size-4" />
											</div>
											<div className="flex flex-col">
												<span className="text-sm font-medium text-content-primary">
													{chat.title || "Untitled Chat"}
												</span>
												<span className="text-xs text-content-secondary">
													{chat.provider} · {chat.model}
												</span>
											</div>
										</div>
										<span className="text-xs text-content-secondary">
											{new Date(chat.created_at).toLocaleDateString()}
										</span>
									</Link>
								))}
							</div>
						)}
					</section>
				</main>
			</Margins>
		</>
	);
};

export default ChatsPage;
