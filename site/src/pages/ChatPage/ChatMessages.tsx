import { type Message, useChat } from "@ai-sdk/react";
import { type Theme, keyframes, useTheme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import Paper from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import { getChatMessages } from "api/queries/chats";
import type { ChatMessage, CreateChatMessageRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { SendIcon } from "lucide-react";
import {
	type FC,
	type KeyboardEvent,
	memo,
	useCallback,
	useEffect,
	useRef,
} from "react";
import ReactMarkdown from "react-markdown";
import { useQuery } from "react-query";
import { useLocation, useParams } from "react-router-dom";
import rehypeRaw from "rehype-raw";
import remarkGfm from "remark-gfm";
import type { ChatLandingLocationState } from "./ChatLanding";
import { useChatContext } from "./ChatLayout";
import { ChatToolInvocation } from "./ChatToolInvocation";
import { LanguageModelSelector } from "./LanguageModelSelector";

const fadeIn = keyframes`
	from {
		opacity: 0;
		transform: translateY(5px);
	}
	to {
		opacity: 1;
		transform: translateY(0);
	}
`;

const renderReasoning = (reasoning: string, theme: Theme) => (
	<div
		css={{
			marginTop: theme.spacing(1),
			marginLeft: theme.spacing(2),
			borderLeft: `2px solid ${theme.palette.grey[400]}`,
			paddingLeft: theme.spacing(1.5),
			fontStyle: "italic",
			color: theme.palette.text.secondary,
			animation: `${fadeIn} 0.3s ease-out`,
			fontSize: "0.875em",
		}}
	>
		<div
			css={{
				color: theme.palette.grey[700],
				fontWeight: 500,
				marginBottom: theme.spacing(0.5),
			}}
		>
			💭 Reasoning:
		</div>
		<div
			css={{
				whiteSpace: "pre-wrap",
				backgroundColor: theme.palette.action.hover,
				padding: theme.spacing(1.5),
				borderRadius: "6px",
				fontSize: "0.95em",
				lineHeight: 1.5,
			}}
		>
			{reasoning}
		</div>
	</div>
);

interface MessageBubbleProps {
	message: Message;
}

const MessageBubble: FC<MessageBubbleProps> = memo(({ message }) => {
	const theme = useTheme();
	const isUser = message.role === "user";

	return (
		<div
			css={{
				display: "flex",
				justifyContent: isUser ? "flex-end" : "flex-start",
				maxWidth: "80%",
				marginLeft: isUser ? "auto" : 0,
				animation: `${fadeIn} 0.3s ease-out`,
			}}
		>
			<Paper
				elevation={isUser ? 1 : 0}
				variant={isUser ? "elevation" : "outlined"}
				css={{
					padding: theme.spacing(1.25, 1.75),
					fontSize: "0.925rem",
					lineHeight: 1.5,
					backgroundColor: isUser
						? theme.palette.grey[900]
						: theme.palette.background.paper,
					borderColor: !isUser ? theme.palette.divider : undefined,
					color: isUser ? theme.palette.grey[50] : theme.palette.text.primary,
					borderRadius: "16px",
					borderBottomRightRadius: isUser ? "4px" : "16px",
					borderBottomLeftRadius: isUser ? "16px" : "4px",
					width: "auto",
					maxWidth: "100%",
					"& img": {
						maxWidth: "100%",
						maxHeight: "400px",
						height: "auto",
						borderRadius: "8px",
						marginTop: theme.spacing(1),
						marginBottom: theme.spacing(1),
					},
					"& p": {
						margin: theme.spacing(1, 0),
						"&:first-of-type": {
							marginTop: 0,
						},
						"&:last-of-type": {
							marginBottom: 0,
						},
					},
					"& ul, & ol": {
						margin: theme.spacing(1.5, 0),
						paddingLeft: theme.spacing(3),
					},
					"& li": {
						margin: theme.spacing(0.5, 0),
					},
					"& code:not(pre > code)": {
						backgroundColor: isUser
							? theme.palette.grey[700]
							: theme.palette.action.hover,
						color: isUser ? theme.palette.grey[50] : theme.palette.text.primary,
						padding: theme.spacing(0.25, 0.75),
						borderRadius: "4px",
						fontSize: "0.875em",
						fontFamily: "monospace",
					},
					"& pre": {
						backgroundColor: isUser
							? theme.palette.common.black
							: theme.palette.grey[100],
						color: isUser
							? theme.palette.grey[100]
							: theme.palette.text.primary,
						padding: theme.spacing(1.5),
						borderRadius: "8px",
						overflowX: "auto",
						margin: theme.spacing(1.5, 0),
						width: "100%",
						"& code": {
							backgroundColor: "transparent",
							padding: 0,
							fontSize: "0.875em",
							fontFamily: "monospace",
							color: "inherit",
						},
					},
					"& a": {
						color: isUser
							? theme.palette.grey[100]
							: theme.palette.primary.main,
						textDecoration: "underline",
						fontWeight: 500,
						"&:hover": {
							textDecoration: "none",
							color: isUser
								? theme.palette.grey[300]
								: theme.palette.primary.dark,
						},
					},
				}}
			>
				{message.role === "assistant" && message.parts ? (
					<div>
						{message.parts.map((part) => {
							switch (part.type) {
								case "text":
									return (
										<ReactMarkdown
											key={message.id}
											remarkPlugins={[remarkGfm]}
											rehypePlugins={[rehypeRaw]}
											css={{
												"& pre": {
													backgroundColor: theme.palette.background.default,
												},
											}}
										>
											{part.text}
										</ReactMarkdown>
									);
								case "tool-invocation":
									return (
										<div key={message.id}>
											<ChatToolInvocation
												toolInvocation={
													part.toolInvocation as ChatToolInvocation
												}
											/>
										</div>
									);
								case "reasoning":
									return (
										<div key={message.id}>
											{renderReasoning(part.reasoning, theme)}
										</div>
									);
								default:
									return null;
							}
						})}
					</div>
				) : (
					<ReactMarkdown
						remarkPlugins={[remarkGfm]}
						rehypePlugins={[rehypeRaw]}
					>
						{message.content}
					</ReactMarkdown>
				)}
			</Paper>
		</div>
	);
});

interface ChatViewProps {
	messages: Message[];
	input: string;
	handleInputChange: React.ChangeEventHandler<
		HTMLInputElement | HTMLTextAreaElement
	>;
	handleSubmit: (e?: React.FormEvent<HTMLFormElement>) => void;
	isLoading: boolean;
	chatID: string;
}

const ChatView: FC<ChatViewProps> = ({
	messages,
	input,
	handleInputChange,
	handleSubmit,
	isLoading,
}) => {
	const theme = useTheme();
	const messagesEndRef = useRef<HTMLDivElement>(null);
	const inputRef = useRef<HTMLTextAreaElement>(null);
	const chatContext = useChatContext();

	useEffect(() => {
		const timer = setTimeout(() => {
			messagesEndRef.current?.scrollIntoView({
				behavior: "smooth",
				block: "end",
			});
		}, 50);
		return () => clearTimeout(timer);
	}, []);

	useEffect(() => {
		inputRef.current?.focus();
	}, []);

	const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
		if (event.key === "Enter" && !event.shiftKey) {
			event.preventDefault();
			handleSubmit();
		}
	};

	return (
		<div
			css={{
				display: "flex",
				flexDirection: "column",
				height: "100%",
				backgroundColor: theme.palette.background.default,
			}}
		>
			<div
				css={{
					flexGrow: 1,
					overflowY: "auto",
					padding: theme.spacing(3),
				}}
			>
				<div
					css={{
						maxWidth: "900px",
						width: "100%",
						margin: "0 auto",
						display: "flex",
						flexDirection: "column",
						gap: theme.spacing(3),
					}}
				>
					{messages.map((message) => (
						<MessageBubble key={`message-${message.id}`} message={message} />
					))}
					<div ref={messagesEndRef} />
				</div>
			</div>

			<div
				css={{
					width: "100%",
					maxWidth: "900px",
					margin: "0 auto",
					padding: theme.spacing(2, 3, 2, 3),
					backgroundColor: theme.palette.background.default,
					borderTop: `1px solid ${theme.palette.divider}`,
					flexShrink: 0,
				}}
			>
				<Paper
					component="form"
					onSubmit={handleSubmit}
					elevation={0}
					variant="outlined"
					css={{
						padding: theme.spacing(0.5, 0.5, 0.5, 1.5),
						display: "flex",
						alignItems: "flex-start",
						width: "100%",
						borderRadius: "12px",
						backgroundColor: theme.palette.background.paper,
						transition: "border-color 0.2s ease",
						"&:focus-within": {
							borderColor: theme.palette.primary.main,
						},
					}}
				>
					<div
						css={{
							marginRight: theme.spacing(1),
							alignSelf: "flex-end",
							marginBottom: theme.spacing(0.5),
						}}
					>
						<LanguageModelSelector />
					</div>
					<TextField
						inputRef={inputRef}
						value={input}
						disabled={isLoading || chatContext.selectedModel === ""}
						onChange={handleInputChange}
						onKeyDown={handleKeyDown}
						placeholder="Ask Coder..."
						fullWidth
						variant="standard"
						multiline
						maxRows={5}
						InputProps={{ disableUnderline: true }}
						css={{
							alignSelf: "center",
							padding: theme.spacing(0.75, 0),
							fontSize: "0.9rem",
						}}
						autoFocus
					/>
					<IconButton
						type="submit"
						color="primary"
						disabled={
							!input.trim() || isLoading || chatContext.selectedModel === ""
						}
						css={{
							alignSelf: "flex-end",
							marginBottom: theme.spacing(0.5),
							transition: "transform 0.2s ease, background-color 0.2s ease",
							"&:not(:disabled):hover": {
								transform: "scale(1.1)",
								backgroundColor: theme.palette.action.hover,
							},
						}}
					>
						<SendIcon />
					</IconButton>
				</Paper>
			</div>
		</div>
	);
};

export const ChatMessages: FC = () => {
	const { chatID } = useParams();
	if (!chatID) {
		throw new Error("Chat ID is required in URL path /chat/:chatID");
	}

	const { state } = useLocation();
	const transferredState = state as ChatLandingLocationState | undefined;

	const messagesQuery = useQuery<ChatMessage[], Error>(getChatMessages(chatID));

	const chatContext = useChatContext();

	const {
		messages,
		input,
		handleInputChange,
		handleSubmit: originalHandleSubmit,
		isLoading,
		setInput,
		setMessages,
	} = useChat({
		id: chatID,
		api: `/api/v2/chats/${chatID}/messages`,
		experimental_prepareRequestBody: (options): CreateChatMessageRequest => {
			const userMessages = options.messages.filter(
				(message) => message.role === "user",
			);
			const mostRecentUserMessage = userMessages.at(-1);
			return {
				model: chatContext.selectedModel,
				message: mostRecentUserMessage,
				thinking: false,
			};
		},
		initialInput: transferredState?.message,
		initialMessages: messagesQuery.data as Message[] | undefined,
	});

	// Update messages from query data when it loads
	useEffect(() => {
		if (messagesQuery.data && messages.length === 0) {
			setMessages(messagesQuery.data as Message[]);
		}
	}, [messagesQuery.data, messages.length, setMessages]);

	const handleSubmitCallback = useCallback(
		(e?: React.FormEvent<HTMLFormElement>) => {
			if (e) e.preventDefault();
			if (!input.trim()) return;
			originalHandleSubmit();
			setInput(""); // Clear input after submit
		},
		[input, originalHandleSubmit, setInput],
	);

	// Clear input and potentially submit on initial load with message
	useEffect(() => {
		if (transferredState?.message && input === transferredState.message) {
			// Prevent submitting if messages already exist (e.g., browser back/forward)
			if (messages.length === (messagesQuery.data?.length ?? 0)) {
				handleSubmitCallback(); // Use the correct callback name
			}
			// Clear the state to prevent re-submission on subsequent renders/navigation
			window.history.replaceState({}, document.title);
		}
	}, [
		transferredState?.message,
		input,
		handleSubmitCallback,
		messages.length,
		messagesQuery.data?.length,
	]); // Use the correct callback name

	useEffect(() => {
		if (transferredState?.message) {
			// Logic potentially related to transferredState can go here if needed,
		}
	}, [transferredState?.message]);

	if (messagesQuery.error) {
		return <ErrorAlert error={messagesQuery.error} />;
	}

	if (messagesQuery.isLoading && messages.length === 0) {
		return <Loader fullscreen />;
	}

	return (
		<ChatView
			key={chatID}
			chatID={chatID}
			messages={messages}
			input={input}
			handleInputChange={handleInputChange}
			handleSubmit={handleSubmitCallback}
			isLoading={isLoading}
		/>
	);
};
