import "@xterm/xterm/css/xterm.css";
import type { Interpolation, Theme } from "@emotion/react";
import { CanvasAddon } from "@xterm/addon-canvas";
import { FitAddon } from "@xterm/addon-fit";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { Terminal as XTermTerminal } from "@xterm/xterm";
import { deploymentConfig } from "api/queries/deployment";
import { appearanceSettings } from "api/queries/users";
import { useProxy } from "contexts/ProxyContext";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import themes from "theme";
import { DEFAULT_TERMINAL_FONT, terminalFonts } from "theme/constants";
import { openMaybePortForwardedURL } from "utils/portForward";
import { terminalWebsocketUrl } from "utils/terminal";
import {
	ExponentialBackoff,
	type Websocket,
	WebsocketBuilder,
	WebsocketEvent,
} from "websocket-ts";
import type { ConnectionStatus } from "./types";

/**
 * @public
 */
export interface TerminalProps {
	agentId: string;
	agentName?: string;
	agentOS?: string;
	workspaceName: string;
	username: string;
	reconnectionToken: string;
	command?: string;
	containerName?: string;
	containerUser?: string;
	onConnectionStatus?: (status: ConnectionStatus) => void;
	className?: string;
}

export const Terminal: FC<TerminalProps> = ({
	agentId,
	agentName,
	agentOS,
	workspaceName,
	username,
	reconnectionToken,
	command,
	containerName,
	containerUser,
	onConnectionStatus,
	className,
}) => {
	const theme = themes.dark;
	const { proxy } = useProxy();
	const terminalWrapperRef = useRef<HTMLDivElement>(null);
	const [terminal, setTerminal] = useState<XTermTerminal>();
	const [connectionStatus, setConnectionStatus] =
		useState<ConnectionStatus>("initializing");

	const config = useQuery(deploymentConfig());
	const renderer = config.data?.config.web_terminal_renderer;

	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const currentTerminalFont =
		appearanceSettingsQuery.data?.terminal_font || DEFAULT_TERMINAL_FONT;

	// Notify parent of connection status changes
	useEffect(() => {
		onConnectionStatus?.(connectionStatus);
	}, [connectionStatus, onConnectionStatus]);

	// handleWebLink handles opening of URLs in the terminal
	const handleWebLink = useCallback(
		(uri: string) => {
			openMaybePortForwardedURL(
				uri,
				proxy.preferredWildcardHostname,
				agentName,
				workspaceName,
				username,
			);
		},
		[agentName, workspaceName, username, proxy.preferredWildcardHostname],
	);
	const handleWebLinkRef = useRef(handleWebLink);
	useEffect(() => {
		handleWebLinkRef.current = handleWebLink;
	}, [handleWebLink]);

	// Create the terminal
	const fitAddonRef = useRef<FitAddon | undefined>(undefined);
	const websocketRef = useRef<Websocket | undefined>(undefined);

	useEffect(() => {
		if (!terminalWrapperRef.current || config.isLoading) {
			return;
		}

		const xterm = new XTermTerminal({
			allowProposedApi: true,
			allowTransparency: true,
			disableStdin: false,
			fontFamily: terminalFonts[currentTerminalFont],
			fontSize: 16,
			theme: {
				background: theme.palette.background.default,
			},
		});

		if (renderer === "webgl") {
			try {
				xterm.loadAddon(new WebglAddon());
			} catch {
				// Fallback to canvas if WebGL fails
				xterm.loadAddon(new CanvasAddon());
			}
		} else if (renderer === "canvas") {
			xterm.loadAddon(new CanvasAddon());
		}

		const fitAddon = new FitAddon();
		fitAddonRef.current = fitAddon;
		xterm.loadAddon(fitAddon);
		xterm.loadAddon(new Unicode11Addon());
		xterm.unicode.activeVersion = "11";
		xterm.loadAddon(
			new WebLinksAddon((_, uri) => {
				handleWebLinkRef.current(uri);
			}),
		);

		// Make shift+enter send escaped carriage return
		const escapedCarriageReturn = "\x1b\r";
		xterm.attachCustomKeyEventHandler((ev) => {
			if (ev.shiftKey && ev.key === "Enter") {
				if (ev.type === "keydown") {
					websocketRef.current?.send(
						new TextEncoder().encode(
							JSON.stringify({ data: escapedCarriageReturn }),
						),
					);
				}
				return false;
			}
			return true;
		});

		xterm.open(terminalWrapperRef.current);

		// Fit twice to avoid overflow issues
		fitAddon.fit();
		fitAddon.fit();

		const listener = () => fitAddon.fit();
		window.addEventListener("resize", listener);

		setTerminal(xterm);

		return () => {
			window.removeEventListener("resize", listener);
			xterm.dispose();
		};
	}, [
		config.isLoading,
		renderer,
		theme.palette.background.default,
		currentTerminalFont,
	]);

	// Hook up the terminal through WebSocket
	useEffect(() => {
		if (!terminal) {
			return;
		}

		terminal.clear();
		terminal.focus();
		terminal.options.disableStdin = true;

		let websocket: Websocket | null;
		const disposers = [
			terminal.onData((data) => {
				websocket?.send(
					new TextEncoder().encode(JSON.stringify({ data: data })),
				);
			}),
			terminal.onResize((event) => {
				websocket?.send(
					new TextEncoder().encode(
						JSON.stringify({
							height: event.rows,
							width: event.cols,
						}),
					),
				);
			}),
		];

		let disposed = false;

		terminalWebsocketUrl(
			process.env.NODE_ENV !== "development"
				? proxy.preferredPathAppURL
				: undefined,
			reconnectionToken,
			agentId,
			command,
			terminal.rows,
			terminal.cols,
			containerName,
			containerUser,
		)
			.then((url) => {
				if (disposed) {
					return;
				}

				websocket = new WebsocketBuilder(url)
					.withBackoff(new ExponentialBackoff(1000, 6))
					.build();
				websocket.binaryType = "arraybuffer";
				websocketRef.current = websocket;

				websocket.addEventListener(WebsocketEvent.open, () => {
					terminal.options = {
						disableStdin: false,
						windowsMode: agentOS === "windows",
					};
					websocket?.send(
						new TextEncoder().encode(
							JSON.stringify({
								height: terminal.rows,
								width: terminal.cols,
							}),
						),
					);
					setConnectionStatus("connected");
				});

				websocket.addEventListener(WebsocketEvent.error, (_, event) => {
					console.error("WebSocket error:", event);
					terminal.options.disableStdin = true;
					setConnectionStatus("disconnected");
				});

				websocket.addEventListener(WebsocketEvent.close, () => {
					terminal.options.disableStdin = true;
					setConnectionStatus("disconnected");
				});

				websocket.addEventListener(WebsocketEvent.message, (_, event) => {
					if (typeof event.data === "string") {
						terminal.write(event.data);
					} else {
						terminal.write(new Uint8Array(event.data));
					}
				});

				websocket.addEventListener(WebsocketEvent.reconnect, () => {
					if (websocket) {
						websocket.binaryType = "arraybuffer";
						websocket.send(
							new TextEncoder().encode(
								JSON.stringify({
									height: terminal.rows,
									width: terminal.cols,
								}),
							),
						);
					}
				});
			})
			.catch((error) => {
				if (disposed) {
					return;
				}
				console.error("WebSocket connection failed:", error);
				setConnectionStatus("disconnected");
			});

		return () => {
			disposed = true;
			for (const d of disposers) {
				d.dispose();
			}
			websocket?.close(1000);
			websocketRef.current = undefined;
		};
	}, [
		command,
		proxy.preferredPathAppURL,
		terminal,
		agentId,
		agentOS,
		containerName,
		containerUser,
		reconnectionToken,
	]);

	return (
		<div
			css={styles.terminal}
			ref={terminalWrapperRef}
			data-testid="terminal"
			data-status={connectionStatus}
			className={className}
		/>
	);
};

const styles = {
	terminal: (theme) => ({
		width: "100%",
		height: "100%",
		overflow: "hidden",
		backgroundColor: theme.palette.background.paper,
		"& .xterm": {
			padding: 4,
			width: "100%",
			height: "100%",
		},
		"& .xterm-viewport": {
			width: "auto !important",
		},
		"& .xterm-viewport::-webkit-scrollbar": {
			width: "10px",
		},
		"& .xterm-viewport::-webkit-scrollbar-track": {
			backgroundColor: "inherit",
		},
		"& .xterm-viewport::-webkit-scrollbar-thumb": {
			minHeight: 20,
			backgroundColor: "rgba(255, 255, 255, 0.18)",
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
