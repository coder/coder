import { PanelRightIcon } from "lucide-react";
import { type ReactNode, useContext } from "react";
import type { ChatMCPApp } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import {
	formatModelIntentLabel,
	type ToolStatus,
} from "../ChatElements/tools/utils";
import { MCPAppContext } from "./MCPAppContext";
import { MCPAppFrame } from "./MCPAppFrame";
import { mcpAppResourceURL } from "./resourceURL";

interface MCPAppToolProps {
	app: ChatMCPApp;
	toolCallId: string;
	name: string;
	status?: ToolStatus;
	isError?: boolean;
	modelIntent?: string;
	args: unknown;
	fallback: ReactNode;
}

export const MCPAppTool = ({
	app,
	toolCallId,
	name,
	status = "completed",
	isError = false,
	modelIntent,
	args,
	fallback,
}: MCPAppToolProps) => {
	const { experiments } = useDashboard();
	const { chatId, onOpenApp } = useContext(MCPAppContext);
	if (!experiments.includes("chat-mcp-apps") || !chatId) return fallback;
	const src = mcpAppResourceURL(chatId, app.server_name, app.resource_uri);
	return (
		<ToolCall.Root
			status={status}
			isError={isError}
			hasContent={false}
			className="w-full overflow-hidden rounded-lg border border-border-default"
		>
			<div className="flex items-center justify-between gap-2 border-b border-border-default px-3 py-2">
				<ToolCall.Header
					iconName={name}
					label={modelIntent ? formatModelIntentLabel(modelIntent) : name}
				/>
				{onOpenApp && (
					<Button
						size="sm"
						variant="subtle"
						onClick={() =>
							onOpenApp({
								id: `mcp-app-${toolCallId}`,
								kind: "mcp_app",
								label: name,
								toolCallId,
								serverName: app.server_name,
								resourceUri: app.resource_uri,
							})
						}
					>
						<PanelRightIcon />
						Open in panel
					</Button>
				)}
			</div>
			<MCPAppFrame
				key={src}
				src={src}
				title={name}
				args={args}
				result={app.result}
				displayMode="inline"
				fallback={fallback}
			/>
		</ToolCall.Root>
	);
};
