import { PanelRightIcon } from "lucide-react";
import { type ReactNode, useContext, useEffect, useRef, useState } from "react";
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
	const frameSlotRef = useRef<HTMLDivElement>(null);
	const [nearViewport, setNearViewport] = useState(false);
	useEffect(() => {
		const slot = frameSlotRef.current;
		if (!slot || nearViewport) return;
		// A long transcript can hold many apps; each one is an untrusted runtime
		// and a resource read through the workspace agent, so mount it only when
		// it comes within one viewport of view.
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) {
					setNearViewport(true);
				}
			},
			{ rootMargin: "100% 0px" },
		);
		observer.observe(slot);
		return () => observer.disconnect();
	}, [nearViewport]);
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
			<div ref={frameSlotRef}>
				{nearViewport ? (
					<MCPAppFrame
						key={src}
						src={src}
						title={name}
						args={args}
						result={app.result}
						displayMode="inline"
						fallback={fallback}
					/>
				) : (
					<div className="h-80" />
				)}
			</div>
		</ToolCall.Root>
	);
};
