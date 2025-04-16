import React, { FC, useMemo } from "react";
import { useTheme } from "@emotion/react";
import type { ToolCall, ToolResult } from "@ai-sdk/provider-utils";
import * as TypesGen from "api/typesGenerated";
import CheckCircle from "@mui/icons-material/CheckCircle";
import CircularProgress from "@mui/material/CircularProgress";
import ErrorIcon from "@mui/icons-material/Error";
import CodeIcon from "@mui/icons-material/Code";
import ArticleIcon from "@mui/icons-material/Article";
import { Tooltip } from "@mui/material";
import { InfoIcon } from "lucide-react";

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

	return (
		<div
			css={{
				marginTop: theme.spacing(1),
				marginLeft: theme.spacing(3),
				borderLeft: `2px solid ${theme.palette.divider}`,
				paddingLeft: theme.spacing(1.5),
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
				<Tooltip title="Content">
					<InfoIcon size={12} color={theme.palette.text.disabled} />
				</Tooltip>
			</div>
			{toolInvocation.state === "result" && (
				<ChatToolInvocationResultPreview toolInvocation={toolInvocation} />
			)}
		</div>
	);
};

const ChatToolInvocationResultPreview: FC<{
	toolInvocation: Extract<ChatToolInvocation, { state: "result" }>;
}> = ({ toolInvocation }) => {
	const theme = useTheme();

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
				<div>
					{toolInvocation.result.map((workspace) => (
						<div key={workspace.id}>
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
							{workspace.name}
						</div>
					))}
				</div>
			);
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
};

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
