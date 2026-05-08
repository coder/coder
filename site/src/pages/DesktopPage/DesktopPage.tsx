import RFB from "@novnc/novnc/lib/rfb";
import { type FC, useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { watchAgentDesktop } from "#/api/api";
import { workspaceByOwnerAndName } from "#/api/queries/workspaces";
import { Loader } from "#/components/Loader/Loader";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import themes from "#/theme";
import { pageTitle } from "#/utils/page";
import { getMatchingAgentOrFirst } from "#/utils/workspace";

type ConnectionStatus = "initializing" | "connecting" | "connected" | "error";

const DesktopPage: FC = () => {
	const params = useParams() as { username: string; workspace: string };
	const username = params.username.replace("@", "");
	const parts = params.workspace?.split(".");
	const workspace = useQuery(workspaceByOwnerAndName(username, parts?.[0]));
	const agent = workspace.data
		? getMatchingAgentOrFirst(workspace.data, parts?.[1])
		: undefined;

	const containerRef = useRef<HTMLDivElement | null>(null);
	const [status, setStatus] = useState<ConnectionStatus>("initializing");
	const [errorMessage, setErrorMessage] = useState<string | null>(null);

	useEffect(() => {
		const container = containerRef.current;
		if (!agent || !container) {
			return;
		}
		setStatus("connecting");
		setErrorMessage(null);

		const socket = watchAgentDesktop(agent.id);
		let rfb: RFB | null = null;

		// Surface the coderd 403 from the DLP gate. The handler returns
		// a JSON body before upgrading, so the socket fires `close` with
		// code 1006 before `open`. Read the response body via fetch as a
		// fallback to display the policy name to the user.
		const handleSocketClose = async (event: CloseEvent) => {
			if (rfb) {
				return; // Real disconnect once VNC is up; let RFB handle it.
			}
			setStatus("error");
			try {
				const res = await fetch(`/api/v2/workspaceagents/${agent.id}/desktop`, {
					credentials: "include",
				});
				if (res.status === 403) {
					const body = await res.json().catch(() => null);
					setErrorMessage(
						body?.detail || body?.message || "Desktop access is blocked.",
					);
					return;
				}
			} catch {
				// Fall through to generic message.
			}
			setErrorMessage(
				event.reason || "Failed to open desktop WebSocket connection.",
			);
		};
		socket.addEventListener("close", handleSocketClose);

		// Bridge the OS clipboard to the remote VNC clipboard.
		//
		// Server -> client: when the remote desktop emits a CutText, the
		// RFB instance fires a `clipboard` event whose detail.text we
		// mirror to the browser clipboard via writeText. Subject to the
		// browser's secure-context and user-gesture rules; we silently
		// no-op on rejection rather than surfacing toasts the user
		// cannot action (the DLP proxy may even be dropping the message
		// upstream).
		//
		// Client -> server: on tab focus and visibility transitions we
		// read the OS clipboard and forward via rfb.clipboardPasteFrom
		// so the remote sees a ClientCutText shortly after the user
		// copies on their local machine. Permission denial is silently
		// ignored. A `lastSynced` cell tracks the last text we routed
		// in either direction so the two sides do not echo each other
		// into a loop.
		const lastSynced: { text: string | null } = { text: null };
		const handleRemoteClipboard = (event: Event) => {
			const detail = (event as CustomEvent<{ text?: string }>).detail;
			const text = detail?.text;
			if (typeof text !== "string") return;
			if (text === lastSynced.text) return;
			lastSynced.text = text;
			navigator.clipboard?.writeText?.(text).catch(() => {
				// Silent no-op: clipboard permission denied or unsupported.
			});
		};
		const pushLocalClipboard = async () => {
			if (!rfb) return;
			const reader = navigator.clipboard?.readText?.bind(navigator.clipboard);
			if (!reader) return;
			try {
				const text = await reader();
				if (typeof text !== "string") return;
				if (text === lastSynced.text) return;
				lastSynced.text = text;
				rfb.clipboardPasteFrom(text);
			} catch {
				// Silent no-op: clipboard permission denied or unsupported.
			}
		};
		const handleVisibility = () => {
			if (document.visibilityState === "visible") {
				void pushLocalClipboard();
			}
		};

		try {
			rfb = new RFB(container, socket, { shared: true });
			rfb.scaleViewport = true;
			rfb.resizeSession = false;
			rfb.focusOnClick = true;

			rfb.addEventListener("connect", () => {
				setStatus("connected");
			});
			rfb.addEventListener("disconnect", (event) => {
				setStatus("error");
				setErrorMessage(
					event.detail.clean
						? "Desktop connection closed."
						: "Desktop connection lost.",
				);
			});
			rfb.addEventListener("securityfailure", (event) => {
				setStatus("error");
				setErrorMessage(
					event.detail.reason || "Desktop security handshake failed.",
				);
			});
			rfb.addEventListener("clipboard", handleRemoteClipboard);

			window.addEventListener("focus", pushLocalClipboard);
			document.addEventListener("visibilitychange", handleVisibility);
		} catch (err) {
			setStatus("error");
			setErrorMessage(err instanceof Error ? err.message : String(err));
			socket.close();
		}

		return () => {
			socket.removeEventListener("close", handleSocketClose);
			window.removeEventListener("focus", pushLocalClipboard);
			document.removeEventListener("visibilitychange", handleVisibility);
			try {
				rfb?.removeEventListener("clipboard", handleRemoteClipboard);
			} catch {
				// Ignore errors during teardown.
			}
			try {
				rfb?.disconnect();
			} catch {
				// Ignore errors during teardown.
			}
		};
	}, [agent]);

	return (
		<ThemeOverride theme={themes.dark}>
			{workspace.data && (
				<title>
					{pageTitle(
						"Desktop",
						`${workspace.data.owner_name}/${workspace.data.name}`,
					)}
				</title>
			)}
			<div className="flex h-screen w-screen flex-col bg-surface-primary">
				{status !== "connected" && (
					<div className="flex flex-1 items-center justify-center text-content-primary">
						{status === "error" ? (
							<div className="max-w-md text-center">
								<p className="font-semibold">
									Failed to connect to the workspace desktop.
								</p>
								{errorMessage && (
									<p className="mt-2 text-sm text-content-secondary">
										{errorMessage}
									</p>
								)}
							</div>
						) : (
							<Loader />
						)}
					</div>
				)}
				<div
					ref={containerRef}
					className="flex-1"
					style={{ display: status === "connected" ? "block" : "none" }}
				/>
			</div>
		</ThemeOverride>
	);
};

export default DesktopPage;
