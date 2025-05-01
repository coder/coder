import { useTheme } from "@emotion/react";
import SendIcon from "@mui/icons-material/Send";
import Button from "@mui/material/Button";
import IconButton from "@mui/material/IconButton";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import { createChat } from "api/queries/chats";
import type { Chat } from "api/typesGenerated";
import { Margins } from "components/Margins/Margins";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { type FC, type FormEvent, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { LanguageModelSelector } from "./LanguageModelSelector";

export interface ChatLandingLocationState {
	chat: Chat;
	message: string;
}

export const ChatLanding: FC = () => {
	const { user } = useAuthenticated();
	const theme = useTheme();
	const [input, setInput] = useState("");
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createChatMutation = useMutation(createChat(queryClient));

	const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
		setInput(event.target.value);
	};

	// Placeholder submit handler
	const handleFormSubmit = (e: FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		if (!input.trim()) return;
		console.log("Form submitted with input:", input);
		// Actual submission logic will go elsewhere
		setInput(""); // Clear input after submit (optional)

		createChatMutation.mutateAsync().then((chat) => {
			navigate(`/chat/${chat.id}`, {
				state: {
					chat,
					message: input,
				},
			});
		});
	};

	// Placeholder suggestion handler
	const handleSuggestionClick = (suggestion: string) => {
		setInput(suggestion);
		// Optionally trigger focus on the input field here
	};

	return (
		<Margins>
			<div
				css={{
					display: "flex",
					flexDirection: "column",
					marginTop: theme.spacing(24),
					alignItems: "center",
					paddingBottom: theme.spacing(4),
				}}
			>
				{/* Initial Welcome Message Area */}
				<div
					css={{
						flexGrow: 1,
						display: "flex",
						flexDirection: "column",
						justifyContent: "center",
						alignItems: "center",
						gap: theme.spacing(1),
						padding: theme.spacing(1),
						width: "100%",
						maxWidth: "700px",
						marginBottom: theme.spacing(4),
					}}
				>
					<h1
						css={{
							fontSize: theme.typography.h4.fontSize,
							fontWeight: theme.typography.h4.fontWeight,
							lineHeight: theme.typography.h4.lineHeight,
							marginBottom: theme.spacing(1),
							textAlign: "center",
						}}
					>
						Good evening, {user?.name.split(" ")[0]}
					</h1>
					<p
						css={{
							fontSize: theme.typography.h6.fontSize,
							fontWeight: theme.typography.h6.fontWeight,
							lineHeight: theme.typography.h6.lineHeight,
							color: theme.palette.text.secondary,
							textAlign: "center",
							margin: 0,
							maxWidth: "500px",
							marginInline: "auto",
						}}
					>
						How can I help you today?
					</p>
				</div>

				{/* Input Form and Suggestions - Always Visible */}
				<div css={{ width: "100%", maxWidth: "700px", marginTop: "auto" }}>
					<Stack
						direction="row"
						spacing={2}
						justifyContent="center"
						sx={{ mb: 2 }}
					>
						<Button
							variant="outlined"
							onClick={() =>
								handleSuggestionClick("Help me work on issue #...")
							}
						>
							Work on Issue
						</Button>
						<Button
							variant="outlined"
							onClick={() =>
								handleSuggestionClick("Help me build a template for...")
							}
						>
							Build a Template
						</Button>
						<Button
							variant="outlined"
							onClick={() =>
								handleSuggestionClick("Help me start a new project using...")
							}
						>
							Start a Project
						</Button>
					</Stack>
					<LanguageModelSelector />
					<Paper
						component="form"
						onSubmit={handleFormSubmit}
						elevation={2}
						css={{
							padding: "16px",
							display: "flex",
							alignItems: "center",
							width: "100%",
							borderRadius: "12px",
							border: `1px solid ${theme.palette.divider}`,
						}}
					>
						<TextField
							value={input}
							onChange={handleInputChange}
							placeholder="Ask Coder..."
							fullWidth
							variant="outlined"
							multiline
							maxRows={5}
							css={{
								marginRight: theme.spacing(1),
								"& .MuiOutlinedInput-root": {
									borderRadius: "8px",
									padding: "10px 14px",
								},
							}}
							autoFocus
						/>
						<IconButton type="submit" color="primary" disabled={!input.trim()}>
							<SendIcon />
						</IconButton>
					</Paper>
				</div>
			</div>
		</Margins>
	);
};

export default ChatLanding;
