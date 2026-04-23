import "@xterm/xterm/css/xterm.css";
import { CanvasAddon } from "@xterm/addon-canvas";
import { FitAddon } from "@xterm/addon-fit";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { Terminal } from "@xterm/xterm";
import {
	type Ref,
	useCallback,
	useEffect,
	useEffectEvent,
	useId,
	useImperativeHandle,
	useRef,
	useState,
} from "react";
import {
	ExponentialBackoff,
	type Websocket,
	WebsocketBuilder,
	WebsocketEvent,
} from "websocket-ts";
import { useClipboard } from "#/hooks/useClipboard";
import { cn } from "#/utils/cn";
import { terminalWebsocketUrl } from "#/utils/terminal";
import type { ConnectionStatus } from "./types";

export type WorkspaceTerminalHandle = {
	refit: () => void;
};

type WorkspaceTerminalProps = {
	ref?: Ref<WorkspaceTerminalHandle>;
	agentId: string | undefined;
	operatingSystem?: string;
	className?: string;
	autoFocus?: boolean;
	isVisible?: boolean;
	initialCommand?: string;
	containerName?: string;
	containerUser?: string;
	onStatusChange?: (status: ConnectionStatus) => void;
	onError?: (error: Error) => void;
	reconnectionToken: string;
	baseUrl?: string;
	terminalFontFamily?: string;
	renderer?: string;
	backgroundColor?: string;
	onOpenLink?: (uri: string) => void;
	loading?: boolean;
	errorMessage?: string;
	testId?: string;
};

const DEFAULT_TERMINAL_FONT_FAMILY = "monospace";
const ESCAPED_CARRIAGE_RETURN = "\x1b\r";

const encodeTerminalPayload = (payload: Record<string, number | string>) => {
	return new TextEncoder().encode(JSON.stringify(payload));
};

export const WorkspaceTerminal = ({
	ref,
	agentId,
	operatingSystem,
	className,
	autoFocus = true,
	isVisible = true,
	initialCommand,
	containerName,
	containerUser,
	onStatusChange,
	onError,
	reconnectionToken,
	baseUrl,
	terminalFontFamily = DEFAULT_TERMINAL_FONT_FAMILY,
	renderer,
	backgroundColor,
	onOpenLink,
	loading = false,
	errorMessage,
	testId,
}: WorkspaceTerminalProps) => {
	const scopeId = useId();
	const terminalWrapperRef = useRef<HTMLDivElement>(null);
	const fitAddonRef = useRef<FitAddon | undefined>(undefined);
	const websocketRef = useRef<Websocket | undefined>(undefined);
	const handleOpenLink = useEffectEvent((uri: string) => {
		onOpenLink ? onOpenLink(uri) : window.open(uri, "_blank", "noopener");
	});
	const handleStatusChange = useEffectEvent((status: ConnectionStatus) => {
		onStatusChange?.(status);
	});
	const [terminal, setTerminal] = useState<Terminal>();
	const { copyToClipboard } = useClipboard();

	const [hasBeenVisible, setHasBeenVisible] = useState(false);
	if (isVisible && !hasBeenVisible) {
		setHasBeenVisible(true);
	}

	const reportTerminalError = useEffectEvent((error: Error) => {
		console.error(error);
		onError?.(error);
	});

	const getTerminalDimensions = useCallback(
		(terminal: Terminal): { height: number; width: number } | null => {
			if (terminal.rows <= 0 || terminal.cols <= 0) {
				reportTerminalError(
					new Error(
						`Terminal has non-positive dimensions: ${terminal.rows}x${terminal.cols}`,
					),
				);
				return null;
			}

			return {
				height: terminal.rows,
				width: terminal.cols,
			};
		},
		[],
	);

	const refit = useCallback(() => {
		const fitAddon = fitAddonRef.current;
		if (!fitAddon) {
			return;
		}

		// We have to fit twice here. It's unknown why, but the
		// first fit will overflow slightly in some scenarios.
		// Applying a second fit resolves this.
		try {
			fitAddon.fit();
			fitAddon.fit();
		} catch (error) {
			// biome-ignore lint/suspicious/noConsole: Expected transient fit failure while xterm initializes.
			console.debug("Terminal fit skipped: renderer not ready", error);
		}
	}, []);

	useImperativeHandle(
		ref,
		() => ({
			refit,
		}),
		[refit],
	);

	useEffect(() => {
		if (!hasBeenVisible) {
			return;
		}

		const mountNode = terminalWrapperRef.current;
		if (!mountNode) {
			reportTerminalError(new Error("Terminal mount container is unavailable"));
			return;
		}

		const nextTerminal = new Terminal({
			allowProposedApi: true,
			allowTransparency: true,
			disableStdin: false,
			fontFamily: terminalFontFamily,
			fontSize: 16,
			...(backgroundColor ? { theme: { background: backgroundColor } } : {}),
		});

		if (renderer === "webgl") {
			nextTerminal.loadAddon(new WebglAddon());
		} else if (renderer === "canvas") {
			nextTerminal.loadAddon(new CanvasAddon());
		}

		const fitAddon = new FitAddon();
		fitAddonRef.current = fitAddon;
		nextTerminal.loadAddon(fitAddon);
		nextTerminal.loadAddon(new Unicode11Addon());
		nextTerminal.unicode.activeVersion = "11";
		nextTerminal.loadAddon(
			new WebLinksAddon((_, uri) => {
				handleOpenLink(uri);
			}),
		);

		const isMac = navigator.platform.match("Mac");
		const copySelection = () => {
			const selection = nextTerminal.getSelection();
			if (selection) {
				copyToClipboard(selection);
			}
		};

		// There is no way to remove this handler, so we must attach it once and
		// rely on a ref to send it to the current socket.
		nextTerminal.attachCustomKeyEventHandler((event) => {
			// Make shift+enter send ^[^M (escaped carriage return). Applications
			// typically take this to mean to insert a literal newline.
			if (event.shiftKey && event.key === "Enter") {
				if (event.type === "keydown") {
					websocketRef.current?.send(
						encodeTerminalPayload({ data: ESCAPED_CARRIAGE_RETURN }),
					);
				}
				return false;
			}

			// Make ctrl+shift+c (command+shift+c on macOS) copy the selected text.
			// By default this usually launches the browser dev tools, but users
			// expect this keybinding to copy when in the context of the web terminal.
			if (
				(isMac ? event.metaKey : event.ctrlKey) &&
				event.shiftKey &&
				event.key === "C"
			) {
				event.preventDefault();
				if (event.type === "keydown") {
					copySelection();
				}
				return false;
			}

			return true;
		});

		// Browsers don't support automatic copy to the X11 primary
		// selection (highlighted text that can be pasted with
		// middle-click). Instead, copy-on-select writes to the
		// system clipboard. This means users can't middle-click
		// paste in the terminal after selecting, but this tradeoff
		// is necessary because web browsers don't expose primary
		// selection APIs. Most web terminal users expect Ctrl+V or
		// right-click paste anyway.
		nextTerminal.onSelectionChange(() => {
			copySelection();
		});

		nextTerminal.open(mountNode);
		refit();

		window.addEventListener("resize", refit);

		const resizeObserver = new ResizeObserver(() => {
			refit();
		});
		resizeObserver.observe(mountNode);

		setTerminal(nextTerminal);

		return () => {
			window.removeEventListener("resize", refit);
			resizeObserver.disconnect();
			fitAddonRef.current = undefined;
			nextTerminal.dispose();
			setTerminal(undefined);
		};
	}, [
		hasBeenVisible,
		copyToClipboard,
		refit,
		renderer,
		terminalFontFamily,
		backgroundColor,
	]);

	useEffect(() => {
		if (!isVisible) {
			return;
		}

		refit();
	}, [isVisible, refit]);

	useEffect(() => {
		if (!terminal || !isVisible || !autoFocus) {
			return;
		}

		const frame = requestAnimationFrame(() => {
			terminal.focus();
		});

		return () => {
			cancelAnimationFrame(frame);
		};
	}, [terminal, isVisible, autoFocus]);

	useEffect(() => {
		if (!terminal || !hasBeenVisible) {
			return;
		}

		terminal.clear();
		terminal.options.disableStdin = true;

		if (loading) {
			return;
		}

		if (errorMessage) {
			terminal.writeln(errorMessage);
			handleStatusChange("disconnected");
			return;
		}

		if (!agentId) {
			const error = new Error("Terminal requires agentId to connect");
			reportTerminalError(error);
			terminal.writeln(error.message);
			handleStatusChange("disconnected");
			return;
		}

		refit();
		// Fall back to standard dimensions if the terminal hasn't rendered
		// yet (e.g. fit() failed during renderer startup). The correct
		// size will be sent once the ResizeObserver fires.
		const initialDimensions = getTerminalDimensions(terminal) ?? {
			height: 24,
			width: 80,
		};

		let websocket: Websocket | null;
		const disposers = [
			terminal.onData((data) => {
				websocket?.send(encodeTerminalPayload({ data }));
			}),
			terminal.onResize((event) => {
				if (event.rows <= 0 || event.cols <= 0) {
					reportTerminalError(
						new Error(
							`Terminal received non-positive resize: ${event.rows}x${event.cols}`,
						),
					);
					return;
				}

				websocket?.send(
					encodeTerminalPayload({ height: event.rows, width: event.cols }),
				);
			}),
		];

		let disposed = false;
		terminalWebsocketUrl(
			baseUrl,
			reconnectionToken,
			agentId,
			initialCommand,
			initialDimensions.height,
			initialDimensions.width,
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
				const scheduleTerminalResize = () => {
					window.setTimeout(() => {
						if (disposed) {
							return;
						}

						const dimensions = getTerminalDimensions(terminal);
						if (!dimensions) {
							return;
						}

						websocket?.send(
							encodeTerminalPayload({
								height: dimensions.height,
								width: dimensions.width,
							}),
						);
					}, 0);
				};
				websocket.binaryType = "arraybuffer";
				websocketRef.current = websocket;
				websocket.addEventListener(WebsocketEvent.open, () => {
					if (disposed) {
						return;
					}
					terminal.options = {
						disableStdin: false,
						windowsMode: operatingSystem === "windows",
					};
					refit();
					scheduleTerminalResize();
					handleStatusChange("connected");
				});
				websocket.addEventListener(WebsocketEvent.error, (_, event) => {
					if (disposed) {
						return;
					}
					console.error("WebSocket error:", event);
					terminal.options.disableStdin = true;
					handleStatusChange("disconnected");
				});
				websocket.addEventListener(WebsocketEvent.close, () => {
					if (disposed) {
						return;
					}
					terminal.options.disableStdin = true;
					handleStatusChange("disconnected");
				});
				websocket.addEventListener(WebsocketEvent.message, (_, event) => {
					if (disposed) {
						return;
					}
					if (typeof event.data === "string") {
						// This exclusively occurs when testing.
						// "jest-websocket-mock" doesn't support ArrayBuffer.
						terminal.write(event.data);
					} else {
						terminal.write(new Uint8Array(event.data));
					}
				});
				websocket.addEventListener(WebsocketEvent.reconnect, () => {
					if (disposed || !websocket) {
						return;
					}

					websocket.binaryType = "arraybuffer";
					refit();
					const dimensions = getTerminalDimensions(terminal);
					if (!dimensions) {
						return;
					}
					websocket.send(
						encodeTerminalPayload({
							height: dimensions.height,
							width: dimensions.width,
						}),
					);
				});
			})
			.catch((error) => {
				if (disposed) {
					return;
				}
				console.error("WebSocket connection failed:", error);
				reportTerminalError(
					error instanceof Error ? error : new Error(String(error)),
				);
				handleStatusChange("disconnected");
			});

		return () => {
			disposed = true;
			for (const disposer of disposers) {
				disposer.dispose();
			}
			websocket?.close(1000);
			websocketRef.current = undefined;
		};
	}, [
		hasBeenVisible,
		agentId,
		baseUrl,
		containerName,
		containerUser,
		errorMessage,
		getTerminalDimensions,
		initialCommand,
		loading,
		operatingSystem,
		reconnectionToken,
		refit,
		terminal,
	]);

	const terminalScopeSelector = `[data-terminal-scope="${scopeId}"]`;

	return (
		<>
			<style>{`
				${terminalScopeSelector} .xterm {
					padding: 4px;
					width: 100%;
					height: 100%;
				}

				${terminalScopeSelector} .xterm-viewport {
					/* This is required to force full-width on the terminal. */
					/* Otherwise there's a small white bar to the right of the scrollbar. */
					width: auto !important;
				}

				${terminalScopeSelector} .xterm-viewport::-webkit-scrollbar {
					width: 8px;
				}

				${terminalScopeSelector} .xterm-viewport::-webkit-scrollbar-track {
					background-color: transparent;
				}

				${terminalScopeSelector} .xterm-viewport::-webkit-scrollbar-thumb {
					min-height: 20px;
					background-color: hsl(var(--surface-quaternary));
				}
			`}</style>
			<div
				className={cn(
					"workspace-terminal h-full w-full flex-1 min-h-0 overflow-hidden bg-surface-tertiary",
					className,
				)}
				ref={terminalWrapperRef}
				data-terminal-scope={scopeId}
				data-testid={testId}
			/>
		</>
	);
};
