import { Message, useChat } from "@ai-sdk/react";
import { useTheme, Theme, keyframes } from "@emotion/react";
import SendIcon from "@mui/icons-material/Send";
import IconButton from "@mui/material/IconButton";
import Paper, { PaperProps } from "@mui/material/Paper";
import TextField from "@mui/material/TextField";
import { getChatMessages, getChats } from "api/queries/chats";
import { CreateChatMessageRequest, ChatMessage } from "api/typesGenerated";
import { FC, memo, useEffect, useRef, KeyboardEvent } from "react";
import { useQuery, useQueryClient } from "react-query";
import { useLocation, useParams } from "react-router-dom";
import { ChatLandingLocationState } from "./ChatLanding";
import { useChatContext } from "./ChatLayout";
import { LanguageModelSelector } from "./LanguageModelSelector";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";

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

const pulseAnimation = keyframes`
	0% { opacity: 0.6; }
	50% { opacity: 1; }
	100% { opacity: 0.6; }
`;

const renderToolInvocation = (toolInvocation: any, theme: Theme) => (
	<div
		css={{
			marginTop: theme.spacing(1),
			marginLeft: theme.spacing(2),
			borderLeft: `2px solid ${theme.palette.info.light}`,
			paddingLeft: theme.spacing(1.5),
			fontSize: "0.875em",
			fontFamily: "monospace",
			animation: `${fadeIn} 0.3s ease-out`,
		}}
	>
		<div
			css={{
				color: theme.palette.info.light,
				fontStyle: "italic",
				fontWeight: 500,
				marginBottom: theme.spacing(0.5),
			}}
		>
			🛠️ Tool Call: {toolInvocation.toolName}
		</div>
		<div
			css={{
				backgroundColor: theme.palette.action.hover,
				padding: theme.spacing(1.5),
				borderRadius: "6px",
				marginTop: theme.spacing(0.5),
				color: theme.palette.text.secondary,
			}}
		>
			<div css={{ marginBottom: theme.spacing(1) }}>
				Arguments:
				<div
					css={{
						marginTop: theme.spacing(0.5),
						fontFamily: "monospace",
						whiteSpace: "pre-wrap",
						wordBreak: "break-all",
						fontSize: "0.9em",
						color: theme.palette.text.primary,
					}}
				>
					{JSON.stringify(toolInvocation.args, null, 2)}
				</div>
			</div>
			{toolInvocation.result && (
				<div>
					Result:
					<div
						css={{
							marginTop: theme.spacing(0.5),
							fontFamily: "monospace",
							whiteSpace: "pre-wrap",
							wordBreak: "break-all",
							fontSize: "0.9em",
							color: theme.palette.text.primary,
						}}
					>
						{JSON.stringify(toolInvocation.result, null, 2)}
					</div>
				</div>
			)}
		</div>
	</div>
);

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
					backgroundColor: isUser ? theme.palette.grey[900] : theme.palette.background.paper,
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
						color: isUser ? theme.palette.grey[100] : theme.palette.text.primary,
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
							color: isUser ? theme.palette.grey[300] : theme.palette.primary.dark,
						},
					},
				}}
			>
				{message.role === "assistant" && message.parts ? (
					<div>
						{message.parts.map((part, partIndex) => {
							switch (part.type) {
								case "text":
									return (
										<ReactMarkdown
											key={partIndex}
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
										<div key={partIndex}>
											{renderToolInvocation(part.toolInvocation, theme)}
										</div>
									);
								case "reasoning":
									return (
										<div key={partIndex}>
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
	handleInputChange: React.ChangeEventHandler<HTMLInputElement | HTMLTextAreaElement>;
	hhandleSubmit: (e?: React.FormEvent<HTMLFormElement>) => void;
	isLoading: boolean;
	chatID: string;
}

const ChatView: FC<ChatViewProps> = ({ 
	messages, 
	input, 
	handleInputChange, 
	hhandleSubmit, 
	isLoading,
	chatID
}) => {
	const theme = useTheme();
	const messagesEndRef = useRef<HTMLDivElement>(null);
	const inputRef = useRef<HTMLTextAreaElement>(null);

	useEffect(() => {
		const timer = setTimeout(() => {
			messagesEndRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
		}, 50);
		return () => clearTimeout(timer);
	}, [messages, isLoading]);

	useEffect(() => {
		inputRef.current?.focus();
	}, [chatID]);

	const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
		if (event.key === 'Enter' && !event.shiftKey) {
			event.preventDefault();
			hhandleSubmit();
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
						maxWidth: '900px',
						width: '100%',
						margin: '0 auto',
						display: "flex",
						flexDirection: "column",
						gap: theme.spacing(3),
					}}
				>
					{messages.map((message, index) => (
						<MessageBubble key={`message-${index}`} message={message} />
					))}
					{isLoading && (
						<div
							css={{
								display: "flex",
								justifyContent: "flex-start",
								maxWidth: "80%",
								animation: `${fadeIn} 0.3s ease-out`,
							}}
						>
							<Paper
								elevation={1}
								css={{
									padding: theme.spacing(1.5, 2),
									fontSize: "0.95rem",
									backgroundColor: theme.palette.background.paper,
									borderRadius: "16px",
									borderBottomLeftRadius: "4px",
									width: "auto",
									display: "flex",
									alignItems: "center",
									gap: theme.spacing(1),
									animation: `${pulseAnimation} 1.5s ease-in-out infinite`,
								}}
							>
								<Loader size={20} /> Thinking...
							</Paper>
						</div>
					)}
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
					onSubmit={hhandleSubmit}
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
							alignSelf: 'flex-end',
							marginBottom: theme.spacing(0.5),
						}}
					>
						<LanguageModelSelector />
					</div>
					<TextField
						inputRef={inputRef}
						value={input}
						onChange={handleInputChange}
						onKeyDown={handleKeyDown}
						placeholder="Ask Coder..."
						fullWidth
						variant="standard"
						multiline
						maxRows={5}
						InputProps={{ disableUnderline: true }}
						css={{
							alignSelf: 'center',
							padding: theme.spacing(0.75, 0),
							fontSize: "0.9rem",
						}}
						autoFocus
					/>
					<IconButton
						type="submit"
						color="primary"
						disabled={!input.trim() || isLoading}
						css={{
							alignSelf: 'flex-end',
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
	const transferedState = state as ChatLandingLocationState | undefined;

	const messagesQuery = useQuery<ChatMessage[], Error>(getChatMessages(chatID));
	
	const chatContext = useChatContext();

	const {
		messages,
		input,
		handleInputChange,
		handleSubmit: originalHandleSubmit,
		isLoading,
		setInput, 
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
		initialInput: transferedState?.message,
		initialMessages: messagesQuery.data as Message[] | undefined,
	});
    useEffect(() => {
        // console.log(transferedState?.message, input)
        if (transferedState?.message && input === transferedState?.message) {
            // handleSubmit();
        }
    }, [transferedState?.message])

	const handleSubmit = (e?: React.FormEvent<HTMLFormElement>) => {
		if (e) e.preventDefault(); 
		if (!input.trim()) return; 
		originalHandleSubmit(); 
		setInput(''); 
	};

	useEffect(() => {
		if (transferedState?.message) {
		}
	}, [transferedState?.message]);

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
			hhandleSubmit={handleSubmit}
			isLoading={isLoading}
		/>
	);
};

export default ChatMessages;
