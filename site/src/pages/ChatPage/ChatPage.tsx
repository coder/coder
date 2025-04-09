import { FC, FormEvent } from "react";
import { useChat } from "@ai-sdk/react";
import { useTheme } from "@emotion/react";
import { Margins } from "components/Margins/Margins";
import TextField from "@mui/material/TextField";
import Paper from "@mui/material/Paper";
import IconButton from "@mui/material/IconButton";
import SendIcon from "@mui/icons-material/Send";
import CircularProgress from "@mui/material/CircularProgress";
import Button from "@mui/material/Button";
import Stack from "@mui/material/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";

export const ChatPage: FC = () => {
	const { user } = useAuthenticated();

	const theme = useTheme();
	const {
		messages,
		input,
		handleInputChange,
		handleSubmit,
		isLoading,
		setInput,
	} = useChat({
		api: "/api/v2/chat",
	});

	const handleFormSubmit = (e: FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		if (!input.trim() || isLoading) return;
		handleSubmit(e);
	};

	const handleSuggestionClick = (suggestion: string) => {
		setInput(suggestion);
	};

	return (
		<Margins>
			<div
				css={{
					display: "flex",
					flexDirection: "column",
					marginTop: theme.spacing(24),
				}}
			>
				{messages.length === 0 && !isLoading ? (
					<div
						css={{
							flexGrow: 1,
							display: "flex",
							flexDirection: "column",
							justifyContent: "center",
							alignItems: "center",
							gap: theme.spacing(1),
							padding: theme.spacing(1),
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
						<Paper
							component="form"
							onSubmit={handleFormSubmit}
							elevation={2}
							css={{
								padding: "16px",
								display: "flex",
								alignItems: "center",
								width: "100%",
								maxWidth: "700px",
								borderRadius: "12px",
								border: `1px solid ${theme.palette.divider}`,
								margin: theme.spacing(4),
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
										outline: "none",
									},
								}}
								autoFocus
							/>
							<IconButton
								type="submit"
								color="primary"
								disabled={!input.trim()}
							>
								<SendIcon />
							</IconButton>
						</Paper>

						<Stack direction="row" spacing={2} justifyContent="center">
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
					</div>
				) : (
					<>
						<div
							css={{
								flexGrow: 1,
								overflowY: "auto",
								padding: theme.spacing(2),
								display: "flex",
								flexDirection: "column",
								gap: theme.spacing(2),
							}}
						>
							{messages.map((m) => (
								<Paper
									key={m.id}
									elevation={m.role === "user" ? 1 : 2}
									css={{
										padding: "12px 16px",
										alignSelf: m.role === "user" ? "flex-end" : "flex-start",
										backgroundColor:
											m.role === "user"
												? theme.palette.primary.light
												: theme.palette.background.paper,
										color:
											m.role === "user"
												? theme.palette.primary.contrastText
												: theme.palette.text.primary,
										maxWidth: "75%",
										borderRadius:
											m.role === "user"
												? "20px 20px 4px 20px"
												: "20px 20px 20px 4px",
										wordWrap: "break-word",
									}}
								>
									<div
										css={{
											fontSize: theme.typography.body1.fontSize,
											fontWeight: theme.typography.body1.fontWeight,
											lineHeight: theme.typography.body1.lineHeight,
											whiteSpace: "pre-wrap",
										}}
									>
										{m.content}
									</div>
								</Paper>
							))}
							{isLoading && messages.length > 0 && (
								<Paper
									elevation={2}
									css={{
										padding: "12px 16px",
										alignSelf: "flex-start",
										backgroundColor: theme.palette.background.paper,
										maxWidth: "fit-content",
										borderRadius: "20px 20px 20px 4px",
										display: "flex",
										alignItems: "center",
										gap: theme.spacing(1),
									}}
								>
									<CircularProgress size={20} />
									<div
										css={{
											fontSize: theme.typography.body1.fontSize,
											fontWeight: theme.typography.body1.fontWeight,
											lineHeight: theme.typography.body1.lineHeight,
											fontStyle: "italic",
											color: theme.palette.text.secondary,
										}}
									>
										Thinking...
									</div>
								</Paper>
							)}
							{isLoading && messages.length === 0 && (
								<div
									css={{
										display: "flex",
										justifyContent: "center",
										alignItems: "center",
										flexGrow: 1,
									}}
								>
									<CircularProgress />
								</div>
							)}
						</div>

						<form
							onSubmit={handleFormSubmit}
							css={{
								padding: theme.spacing(2),
								borderTop: `1px solid ${theme.palette.divider}`,
								backgroundColor: theme.palette.background.paper,
								flexShrink: 0,
								display: "flex",
								alignItems: "center",
								gap: theme.spacing(1),
							}}
						>
							<TextField
								value={input}
								onChange={handleInputChange}
								placeholder="Send a message..."
								fullWidth
								variant="outlined"
								size="small"
								multiline
								maxRows={5}
								css={{ "& .MuiOutlinedInput-root": { borderRadius: "8px" } }}
								disabled={isLoading}
							/>
							<IconButton
								type="submit"
								color="primary"
								disabled={!input.trim() || isLoading}
							>
								<SendIcon />
							</IconButton>
						</form>
					</>
				)}
			</div>
		</Margins>
	);
};

export default ChatPage;
