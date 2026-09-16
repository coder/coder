import type { Transport } from "@modelcontextprotocol/client";
import { type FC, useEffect, useEffectEvent, useRef } from "react";
import { OriginValidatedTransport } from "./originValidatedTransport";
import { MCP_APP_INNER_SANDBOX } from "./useMcpAppBridge";

interface McpAppFrameProps {
	sandboxUrl: string;
	title: string;
	/**
	 * Receives a transport pinned to this iframe's window and origin once
	 * mounted, and `undefined` on unmount.
	 */
	onTransportChange: (transport: Transport | undefined) => void;
}

/**
 * The sandbox proxy iframe. The proxy document creates the inner view
 * iframe itself, so this element only needs the flags that let the proxy
 * run scripts on its own origin.
 */
export const McpAppFrame: FC<McpAppFrameProps> = ({
	sandboxUrl,
	title,
	onTransportChange,
}) => {
	const frameRef = useRef<HTMLIFrameElement>(null);
	const notifyTransport = useEffectEvent(onTransportChange);
	// One transport per mounted element. A remount (new key) produces a new
	// window, and the cleanup clears the old transport first.
	useEffect(() => {
		const frameWindow = frameRef.current?.contentWindow;
		notifyTransport(
			frameWindow
				? new OriginValidatedTransport(frameWindow, new URL(sandboxUrl).origin)
				: undefined,
		);
		return () => notifyTransport(undefined);
	}, [sandboxUrl]);

	return (
		<iframe
			ref={frameRef}
			src={sandboxUrl}
			title={title}
			sandbox={MCP_APP_INNER_SANDBOX}
			className="size-full border-0 bg-transparent"
		/>
	);
};
