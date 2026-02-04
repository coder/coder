import { chats, createChat } from "api/queries/chats";
import type { Chat, CreateChatRequest } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "components/Dialog/Dialog";
import { displayError } from "components/GlobalSnackbar/utils";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Spinner } from "components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { MessageSquareIcon, PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link } from "components/Link/Link";
import { useNavigate } from "react-router";
import { pageTitle } from "utils/page";

const ChatsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const chatsQuery = useQuery(chats());
	const createChatMutation = useMutation(createChat(queryClient));
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [newChat, setNewChat] = useState<CreateChatRequest>({
		title: "",
		provider: "anthropic",
		model: "claude-opus-4-5",
	});

	const handleCreateChat = async () => {
		try {
			const created = await createChatMutation.mutateAsync(newChat);
			setIsCreateOpen(false);
			setNewChat({
				title: "",
				provider: "anthropic",
				model: "claude-opus-4-5",
			});
			// Navigate to the new chat
			navigate(`/chats/${created.id}`);
		} catch {
			displayError("Failed to create chat");
		}
	};

	return (
		<>
			<title>{pageTitle("Chats")}</title>
			<Margins>
				<PageHeader
					actions={
						<Dialog open={isCreateOpen} onOpenChange={setIsCreateOpen}>
							<DialogTrigger asChild>
								<Button>
									<PlusIcon className="size-4" />
									New Chat
								</Button>
							</DialogTrigger>
							<DialogContent>
								<DialogHeader>
									<DialogTitle>Create New Chat</DialogTitle>
									<DialogDescription>
										Start a new conversation with an AI assistant.
									</DialogDescription>
								</DialogHeader>
								<div className="flex flex-col gap-4 py-4">
									<div className="flex flex-col gap-2">
										<Label htmlFor="title">Title (optional)</Label>
										<Input
											id="title"
											value={newChat.title ?? ""}
											onChange={(e) =>
												setNewChat({ ...newChat, title: e.target.value })
											}
											placeholder="My chat"
										/>
									</div>
									<div className="flex flex-col gap-2">
										<Label htmlFor="provider">Provider</Label>
										<Input
											id="provider"
											value={newChat.provider}
											onChange={(e) =>
												setNewChat({ ...newChat, provider: e.target.value })
											}
											placeholder="anthropic"
										/>
									</div>
									<div className="flex flex-col gap-2">
										<Label htmlFor="model">Model</Label>
										<Input
											id="model"
											value={newChat.model}
											onChange={(e) =>
												setNewChat({ ...newChat, model: e.target.value })
											}
											placeholder="claude-opus-4-5"
										/>
									</div>
								</div>
								<DialogFooter>
									<Button
										variant="outline"
										onClick={() => setIsCreateOpen(false)}
									>
										Cancel
									</Button>
									<Button
										onClick={handleCreateChat}
										disabled={createChatMutation.isPending}
									>
										<Spinner loading={createChatMutation.isPending}>
											<PlusIcon className="size-4" />
										</Spinner>
										Create
									</Button>
								</DialogFooter>
							</DialogContent>
						</Dialog>
					}
				>
					<PageHeaderTitle>Chats</PageHeaderTitle>
					<PageHeaderSubtitle>
						Chat with AI assistants that can execute tools
					</PageHeaderSubtitle>
				</PageHeader>

				<main className="pb-8">
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
						<div className="flex flex-col items-center justify-center py-16 text-content-secondary">
							<MessageSquareIcon className="size-12 mb-4" />
							<p className="text-lg font-medium">No chats yet</p>
							<p className="text-sm">
								Create a new chat to start a conversation.
							</p>
						</div>
					)}

					{chatsQuery.isSuccess && chatsQuery.data.length > 0 && (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Title</TableHead>
									<TableHead>Provider</TableHead>
									<TableHead>Model</TableHead>
									<TableHead>Created</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{chatsQuery.data.map((chat: Chat) => (
									<TableRow key={chat.id}>
										<TableCell>
											<Link href={`/chats/${chat.id}`} showExternalIcon={false}>
												{chat.title || "Untitled Chat"}
											</Link>
										</TableCell>
										<TableCell>{chat.provider}</TableCell>
										<TableCell>
											<code className="text-xs">{chat.model}</code>
										</TableCell>
										<TableCell>
											{new Date(chat.created_at).toLocaleDateString()}
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					)}
				</main>
			</Margins>
		</>
	);
};

export default ChatsPage;
