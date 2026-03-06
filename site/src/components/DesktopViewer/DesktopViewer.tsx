import { type FC, useCallback, useEffect, useRef, useState } from "react";

type DesktopViewerStatus = "connecting" | "connected" | "disconnected";

interface DesktopViewerProps {
	chatId: string;
}

/**
 * DesktopViewer connects to a workspace desktop stream over WebSocket
 * and renders it using noVNC's RFB client. The WebSocket URL is
 * derived from the current page origin and the given chat ID.
 *
 * This component expects `@novnc/novnc` to be available. If it is not
 * installed, the viewer will show a "disconnected" status.
 */
export const DesktopViewer: FC<DesktopViewerProps> = ({ chatId }) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const rfbRef = useRef<unknown>(null);
	const [status, setStatus] = useState<DesktopViewerStatus>("connecting");

	const wsUrl = useCallback(() => {
		const protocol =
			globalThis.location.protocol === "https:" ? "wss" : "ws";
		return `${protocol}://${globalThis.location.host}/api/experimental/chats/${chatId}/desktop`;
	}, [chatId]);

	useEffect(() => {
		const container = containerRef.current;
		if (!container) {
			return;
		}

		let rfb: {
			disconnect: () => void;
			removeEventListener: (event: string, handler: () => void) => void;
			addEventListener: (
				event: string,
				handler: (...args: unknown[]) => void,
			) => void;
			scaleViewport: boolean;
		} | null = null;

		const connect = async () => {
			setStatus("connecting");

			try {
				// Dynamically import noVNC to avoid hard dependency at bundle
				// time. If the package is not installed the import will fail
				// and we fall through to the catch block.
				const { default: RFB } = await import(
					// @ts-expect-error -- @novnc/novnc may not be installed.
					"@novnc/novnc/core/rfb"
				);

				const url = wsUrl();
				rfb = new RFB(container, url) as typeof rfb;
				rfbRef.current = rfb;

				if (rfb) {
					rfb.scaleViewport = true;

					rfb.addEventListener("connect", () => {
						setStatus("connected");
					});

					rfb.addEventListener("disconnect", () => {
						setStatus("disconnected");
					});
				}
			} catch {
				// noVNC is not available — show disconnected state.
				setStatus("disconnected");
			}
		};

		connect();

		return () => {
			if (rfb) {
				try {
					rfb.disconnect();
				} catch {
					// Ignore errors during cleanup.
				}
				rfb = null;
				rfbRef.current = null;
			}
		};
	}, [wsUrl]);

	return (
		<div
			style={{
				width: "100%",
				height: "100%",
				position: "relative",
				background: "#000",
			}}
		>
			<div
				ref={containerRef}
				style={{
					width: "100%",
					height: "100%",
				}}
			/>
			{status !== "connected" && (
				<div
					style={{
						position: "absolute",
						top: 0,
						left: 0,
						right: 0,
						bottom: 0,
						display: "flex",
						alignItems: "center",
						justifyContent: "center",
						color: "#fff",
						fontSize: 14,
					}}
				>
					{status === "connecting"
						? "Connecting to desktop..."
						: "Desktop disconnected"}
				</div>
			)}
		</div>
	);
};
