import type { ToolCall, ToolResult } from "@ai-sdk/provider-utils";
import { useTheme } from "@emotion/react";
import ArticleIcon from "@mui/icons-material/Article";
import BuildIcon from "@mui/icons-material/Build";
import CheckCircle from "@mui/icons-material/CheckCircle";
import CodeIcon from "@mui/icons-material/Code";
import DeleteIcon from "@mui/icons-material/Delete";
import ErrorIcon from "@mui/icons-material/Error";
import FileUploadIcon from "@mui/icons-material/FileUpload";
import ListIcon from "@mui/icons-material/List";
import PersonIcon from "@mui/icons-material/Person";
import SettingsIcon from "@mui/icons-material/Settings";
import TerminalIcon from "@mui/icons-material/Terminal";
import Avatar from "@mui/material/Avatar";
import CircularProgress from "@mui/material/CircularProgress";
import Tooltip from "@mui/material/Tooltip";
import type * as TypesGen from "api/typesGenerated";
import { InfoIcon } from "lucide-react";
import type React from "react";
import { type FC, memo, useMemo, useState } from "react";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { dracula } from "react-syntax-highlighter/dist/cjs/styles/prism";
import { vscDarkPlus } from "react-syntax-highlighter/dist/cjs/styles/prism";
import { TabLink, Tabs, TabsList } from "../../components/Tabs/Tabs";

interface ChatToolInvocationProps {
	toolInvocation: ChatToolInvocation;
}

export const ChatToolInvocation: FC<ChatToolInvocationProps> = ({
	toolInvocation,
}) => {
	const theme = useTheme();
	const friendlyName = useMemo(() => {
		return toolInvocation.toolName
			.replace("coder_", "")
			.replace(/_/g, " ")
			.replace(/\b\w/g, (char) => char.toUpperCase());
	}, [toolInvocation.toolName]);

	const hasError = useMemo(() => {
		if (toolInvocation.state !== "result") {
			return false;
		}
		return (
			typeof toolInvocation.result === "object" &&
			toolInvocation.result !== null &&
			"error" in toolInvocation.result
		);
	}, [toolInvocation]);
	const statusColor = useMemo(() => {
		if (toolInvocation.state !== "result") {
			return theme.palette.info.main;
		}
		return hasError ? theme.palette.error.main : theme.palette.success.main;
	}, [toolInvocation, hasError, theme]);
	const tooltipContent = useMemo(() => {
		return (
			<SyntaxHighlighter
				language="json"
				style={dracula}
				css={{
					maxHeight: 300,
					overflow: "auto",
					fontSize: 14,
					borderRadius: theme.shape.borderRadius,
					padding: theme.spacing(1),
					scrollbarWidth: "thin",
					scrollbarColor: "auto",
				}}
			>
				{JSON.stringify(toolInvocation, null, 2)}
			</SyntaxHighlighter>
		);
	}, [toolInvocation]);

	return (
		<div
			css={{
				marginTop: theme.spacing(1),
				marginBottom: theme.spacing(2),
				display: "flex",
				flexDirection: "column",
				gap: theme.spacing(0.75),
				width: "fit-content",
			}}
		>
			<div
				css={{ display: "flex", alignItems: "center", gap: theme.spacing(1) }}
			>
				{toolInvocation.state !== "result" && (
					<CircularProgress
						size={16}
						css={{
							color: statusColor,
						}}
					/>
				)}
				{toolInvocation.state === "result" ? (
					hasError ? (
						<ErrorIcon sx={{ color: statusColor, fontSize: 16 }} />
					) : (
						<CheckCircle sx={{ color: statusColor, fontSize: 16 }} />
					)
				) : null}
				<div
					css={{
						fontSize: "0.9rem",
						fontWeight: 500,
						color: theme.palette.text.primary,
					}}
				>
					{friendlyName}
				</div>
				<Tooltip title={tooltipContent}>
					<InfoIcon size={12} color={theme.palette.text.disabled} />
				</Tooltip>
			</div>
			{toolInvocation.state === "result" ? (
				<ChatToolInvocationResultPreview toolInvocation={toolInvocation} />
			) : (
				<ChatToolInvocationCallPreview toolInvocation={toolInvocation} />
			)}
		</div>
	);
};

const ChatToolInvocationCallPreview: FC<{
	toolInvocation: Extract<
		ChatToolInvocation,
		{ state: "call" | "partial-call" }
	>;
}> = memo(({ toolInvocation }) => {
	const theme = useTheme();

	let content: React.ReactNode;
	switch (toolInvocation.toolName) {
		case "coder_upload_tar_file":
			content = (
				<FilePreview
					files={toolInvocation.args?.files || {}}
					prefix="Uploading files:"
				/>
			);
			break;
	}

	if (!content) {
		return null;
	}

	return <div css={{ paddingLeft: theme.spacing(3) }}>{content}</div>;
});

const ChatToolInvocationResultPreview: FC<{
	toolInvocation: Extract<ChatToolInvocation, { state: "result" }>;
}> = memo(({ toolInvocation }) => {
	const theme = useTheme();

	if (!toolInvocation.result) {
		return null;
	}

	if (
		typeof toolInvocation.result === "object" &&
		"error" in toolInvocation.result
	) {
		return null;
	}

	let content: React.ReactNode;
	switch (toolInvocation.toolName) {
		case "coder_get_workspace":
		case "coder_create_workspace":
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1.5),
					}}
				>
					{toolInvocation.result.template_icon && (
						<img
							src={toolInvocation.result.template_icon || "/icon/code.svg"}
							alt={toolInvocation.result.template_display_name || "Template"}
							css={{
								width: 32,
								height: 32,
								borderRadius: theme.shape.borderRadius / 2,
								objectFit: "contain",
							}}
						/>
					)}
					<div>
						<div css={{ fontWeight: 500, lineHeight: 1.4 }}>
							{toolInvocation.result.name}
						</div>
						<div
							css={{
								fontSize: "0.875rem",
								color: theme.palette.text.secondary,
								lineHeight: 1.4,
							}}
						>
							{toolInvocation.result.template_display_name}
						</div>
					</div>
				</div>
			);
			break;
		case "coder_list_workspaces":
			content = (
				<div
					css={{
						display: "flex",
						flexDirection: "column",
						gap: theme.spacing(1.5),
					}}
				>
					{toolInvocation.result.map((workspace) => (
						<div
							key={workspace.id}
							css={{
								display: "flex",
								alignItems: "center",
								gap: theme.spacing(1.5),
							}}
						>
							{workspace.template_icon && (
								<img
									src={workspace.template_icon || "/icon/code.svg"}
									alt={workspace.template_display_name || "Template"}
									css={{
										width: 32,
										height: 32,
										borderRadius: theme.shape.borderRadius / 2,
										objectFit: "contain",
									}}
								/>
							)}
							<div>
								<div css={{ fontWeight: 500, lineHeight: 1.4 }}>
									{workspace.name}
								</div>
								<div
									css={{
										fontSize: "0.875rem",
										color: theme.palette.text.secondary,
										lineHeight: 1.4,
									}}
								>
									{workspace.template_display_name}
								</div>
							</div>
						</div>
					))}
				</div>
			);
			break;
		case "coder_list_templates": {
			const templates = toolInvocation.result;
			content = (
				<div
					css={{
						display: "flex",
						flexDirection: "column",
						gap: theme.spacing(1.5),
					}}
				>
					{templates.map((template) => (
						<div
							key={template.id}
							css={{
								display: "flex",
								alignItems: "center",
								gap: theme.spacing(1.5),
							}}
						>
							<CodeIcon sx={{ width: 32, height: 32 }} />
							<div>
								<div css={{ fontWeight: 500, lineHeight: 1.4 }}>
									{template.name}
								</div>
								<div
									css={{
										fontSize: "0.875rem",
										color: theme.palette.text.secondary,
										lineHeight: 1.4,
										whiteSpace: "nowrap",
										overflow: "hidden",
										textOverflow: "ellipsis",
										maxWidth: 200,
									}}
									title={template.description}
								>
									{template.description}
								</div>
							</div>
						</div>
					))}
					{templates.length === 0 && <div>No templates found.</div>}
				</div>
			);
			break;
		}
		case "coder_template_version_parameters": {
			const params = toolInvocation.result;
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1),
						fontSize: "0.875rem",
						color: theme.palette.text.secondary,
					}}
				>
					<SettingsIcon fontSize="small" />
					{params.length > 0
						? `${params.length} parameter(s)`
						: "No parameters"}
				</div>
			);
			break;
		}
		case "coder_get_authenticated_user": {
			const user = toolInvocation.result;
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1.5),
					}}
				>
					<Avatar
						src={user.avatar_url}
						alt={user.username}
						sx={{ width: 32, height: 32 }}
					>
						<PersonIcon />
					</Avatar>
					<div>
						<div css={{ fontWeight: 500, lineHeight: 1.4 }}>
							{user.username}
						</div>
						<div
							css={{
								fontSize: "0.875rem",
								color: theme.palette.text.secondary,
								lineHeight: 1.4,
							}}
						>
							{user.email}
						</div>
					</div>
				</div>
			);
			break;
		}
		case "coder_create_workspace_build": {
			const build = toolInvocation.result;
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1),
						fontSize: "0.875rem",
						color: theme.palette.text.secondary,
					}}
				>
					<BuildIcon fontSize="small" />
					Build #{build.build_number} ({build.transition}) status:{" "}
					{build.status}
				</div>
			);
			break;
		}
		case "coder_create_template_version": {
			const version = toolInvocation.result;
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1),
					}}
				>
					<CodeIcon fontSize="small" />
					<div>
						<div css={{ fontWeight: 500, lineHeight: 1.4 }}>{version.name}</div>
						{version.message && (
							<div
								css={{
									fontSize: "0.875rem",
									color: theme.palette.text.secondary,
									lineHeight: 1.4,
								}}
							>
								{version.message}
							</div>
						)}
					</div>
				</div>
			);
			break;
		}
		case "coder_get_workspace_agent_logs":
		case "coder_get_workspace_build_logs":
		case "coder_get_template_version_logs": {
			const logs = toolInvocation.result;
			if (!logs) {
				console.log(toolInvocation);
			}
			const totalLines = logs.length;
			const maxLinesToShow = 5;
			const lastLogs = logs.slice(-maxLinesToShow);
			const hiddenLines = totalLines - lastLogs.length;

			const totalLinesText = `${totalLines} log line${totalLines !== 1 ? "s" : ""}`;
			const hiddenLinesText =
				hiddenLines > 0
					? `... hiding ${hiddenLines} more line${hiddenLines !== 1 ? "s" : ""} ...`
					: null;

			const logsToShow = hiddenLinesText
				? [hiddenLinesText, ...lastLogs]
				: lastLogs;

			content = (
				<div
					css={{
						display: "flex",
						flexDirection: "column",
						gap: theme.spacing(0.5),
					}}
				>
					<div
						css={{
							display: "flex",
							alignItems: "center",
							gap: theme.spacing(1),
							fontSize: "0.875rem",
							color: theme.palette.text.secondary,
						}}
					>
						<ArticleIcon fontSize="small" />
						Retrieved {totalLinesText}.
					</div>
					{logsToShow.length > 0 && (
						<SyntaxHighlighter
							language="log"
							style={dracula}
							customStyle={{
								fontSize: "0.8rem",
								padding: theme.spacing(1),
								margin: 0,
								maxHeight: 150,
								overflowY: "auto",
								scrollbarWidth: "thin",
								scrollbarColor: "auto",
							}}
							showLineNumbers={false}
							lineNumberStyle={{ display: "none" }}
						>
							{logsToShow.join("\n")}
						</SyntaxHighlighter>
					)}
				</div>
			);
			break;
		}
		case "coder_update_template_active_version":
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1),
						fontSize: "0.875rem",
						color: theme.palette.text.secondary,
					}}
				>
					<SettingsIcon fontSize="small" />
					{toolInvocation.result}
				</div>
			);
			break;
		case "coder_upload_tar_file":
			content = (
				<FilePreview files={toolInvocation.args.files} prefix={`Uploaded!`} />
			);
			break;
		case "coder_create_template": {
			const template = toolInvocation.result;
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1.5),
					}}
				>
					<img
						src={template.icon || "/icon/code.svg"}
						alt={template.display_name || "Template"}
						css={{
							width: 32,
							height: 32,
							borderRadius: theme.shape.borderRadius / 2,
							objectFit: "contain",
						}}
					/>
					<div>
						<div css={{ fontWeight: 500, lineHeight: 1.4 }}>
							{template.name}
						</div>
						<div
							css={{
								fontSize: "0.875rem",
								color: theme.palette.text.secondary,
								lineHeight: 1.4,
							}}
						>
							{template.display_name}
						</div>
					</div>
				</div>
			);
			break;
		}
		case "coder_delete_template":
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1),
						fontSize: "0.875rem",
						color: theme.palette.text.secondary,
					}}
				>
					<DeleteIcon fontSize="small" />
					{toolInvocation.result}
				</div>
			);
			break;
		case "coder_get_template_version": {
			const version = toolInvocation.result;
			content = (
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: theme.spacing(1),
					}}
				>
					<CodeIcon fontSize="small" />
					<div>
						<div css={{ fontWeight: 500, lineHeight: 1.4 }}>{version.name}</div>
						{version.message && (
							<div
								css={{
									fontSize: "0.875rem",
									color: theme.palette.text.secondary,
									lineHeight: 1.4,
								}}
							>
								{version.message}
							</div>
						)}
					</div>
				</div>
			);
			break;
		}
		case "coder_download_tar_file": {
			const files = toolInvocation.result;
			content = <FilePreview files={files} prefix="Files:" />;
			break;
		}
		// Add default case or handle other tools if necessary
	}
	return (
		<div
			css={{
				paddingLeft: theme.spacing(3),
			}}
		>
			{content}
		</div>
	);
});

// New component to preview files with tabs
const FilePreview: FC<{ files: Record<string, string>; prefix?: string }> =
	memo(({ files, prefix }) => {
		const theme = useTheme();
		const [selectedTab, setSelectedTab] = useState(0);
		const fileEntries = useMemo(() => Object.entries(files), [files]);

		if (fileEntries.length === 0) {
			return null;
		}

		const handleTabChange = (index: number) => {
			setSelectedTab(index);
		};

		const getLanguage = (filename: string): string => {
			if (filename.includes("Dockerfile")) {
				return "dockerfile";
			}
			const extension = filename.split(".").pop()?.toLowerCase();
			switch (extension) {
				case "tf":
					return "hcl";
				case "json":
					return "json";
				case "yaml":
				case "yml":
					return "yaml";
				case "js":
				case "jsx":
					return "javascript";
				case "ts":
				case "tsx":
					return "typescript";
				case "py":
					return "python";
				case "go":
					return "go";
				case "rb":
					return "ruby";
				case "java":
					return "java";
				case "sh":
					return "bash";
				case "md":
					return "markdown";
				default:
					return "plaintext";
			}
		};

		// Get filename and content based on the selectedTab index
		const [selectedFilename, selectedContent] = fileEntries[selectedTab] ?? [
			"",
			"",
		];

		return (
			<div
				css={{
					display: "flex",
					flexDirection: "column",
					gap: theme.spacing(1),
					width: "100%",
					maxWidth: 400,
				}}
			>
				{prefix && (
					<div
						css={{
							display: "flex",
							alignItems: "center",
							gap: theme.spacing(1),
							fontSize: "0.875rem",
							color: theme.palette.text.secondary,
						}}
					>
						<FileUploadIcon fontSize="small" />
						{prefix}
					</div>
				)}
				{/* Use custom Tabs component with active prop */}
				<Tabs active={selectedFilename} className="flex-shrink-0">
					<TabsList>
						{fileEntries.map(([filename], index) => (
							<TabLink
								key={filename}
								value={filename} // This matches the 'active' prop on Tabs
								to="" // Dummy link, not navigating
								css={{ whiteSpace: "nowrap" }} // Prevent wrapping
								onClick={(e) => {
									e.preventDefault(); // Prevent any potential default link behavior
									handleTabChange(index);
								}}
							>
								{filename}
							</TabLink>
						))}
					</TabsList>
				</Tabs>
				<SyntaxHighlighter
					language={getLanguage(selectedFilename)}
					style={vscDarkPlus}
					customStyle={{
						fontSize: "0.8rem",
						padding: theme.spacing(1),
						margin: 0,
						maxHeight: 200,
						overflowY: "auto",
						scrollbarWidth: "thin",
						scrollbarColor: "auto",
						border: `1px solid ${theme.palette.divider}`,
						borderRadius: theme.shape.borderRadius,
					}}
					showLineNumbers={false}
					lineNumberStyle={{ display: "none" }}
				>
					{selectedContent}
				</SyntaxHighlighter>
			</div>
		);
	});

export type ChatToolInvocation =
	| ToolInvocation<
			"coder_get_workspace",
			{
				id: string;
			},
			TypesGen.Workspace
	  >
	| ToolInvocation<
			"coder_create_workspace",
			{
				user: string;
				template_version_id: string;
				name: string;
				rich_parameters: Record<string, any>;
			},
			TypesGen.Workspace
	  >
	| ToolInvocation<
			"coder_list_workspaces",
			{
				owner: string;
			},
			Pick<
				TypesGen.Workspace,
				| "id"
				| "name"
				| "template_id"
				| "template_name"
				| "template_display_name"
				| "template_icon"
				| "template_active_version_id"
				| "outdated"
			>[]
	  >
	| ToolInvocation<
			"coder_list_templates",
			{},
			Pick<
				TypesGen.Template,
				| "id"
				| "name"
				| "description"
				| "active_version_id"
				| "active_user_count"
			>[]
	  >
	| ToolInvocation<
			"coder_template_version_parameters",
			{
				template_version_id: string;
			},
			TypesGen.TemplateVersionParameter[]
	  >
	| ToolInvocation<"coder_get_authenticated_user", {}, TypesGen.User>
	| ToolInvocation<
			"coder_create_workspace_build",
			{
				workspace_id: string;
				transition: "start" | "stop" | "delete";
			},
			TypesGen.WorkspaceBuild
	  >
	| ToolInvocation<
			"coder_create_template_version",
			{
				template_id?: string;
				file_id: string;
			},
			TypesGen.TemplateVersion
	  >
	| ToolInvocation<
			"coder_get_workspace_agent_logs",
			{
				workspace_agent_id: string;
			},
			string[]
	  >
	| ToolInvocation<
			"coder_get_workspace_build_logs",
			{
				workspace_build_id: string;
			},
			string[]
	  >
	| ToolInvocation<
			"coder_get_template_version_logs",
			{
				template_version_id: string;
			},
			string[]
	  >
	| ToolInvocation<
			"coder_get_template_version",
			{
				template_version_id: string;
			},
			TypesGen.TemplateVersion
	  >
	| ToolInvocation<
			"coder_download_tar_file",
			{
				file_id: string;
			},
			Record<string, string>
	  >
	| ToolInvocation<
			"coder_update_template_active_version",
			{
				template_id: string;
				template_version_id: string;
			},
			string
	  >
	| ToolInvocation<
			"coder_upload_tar_file",
			{
				mime_type: string;
				files: Record<string, string>;
			},
			TypesGen.UploadResponse
	  >
	| ToolInvocation<
			"coder_create_template",
			{
				name: string;
			},
			TypesGen.Template
	  >
	| ToolInvocation<
			"coder_delete_template",
			{
				template_id: string;
			},
			string
	  >;

type ToolInvocation<N extends string, A, R> =
	| ({
			state: "partial-call";
			step?: number;
	  } & ToolCall<N, A>)
	| ({
			state: "call";
			step?: number;
	  } & ToolCall<N, A>)
	| ({
			state: "result";
			step?: number;
	  } & ToolResult<
			N,
			A,
			| R
			| {
					error: string;
			  }
	  >);
