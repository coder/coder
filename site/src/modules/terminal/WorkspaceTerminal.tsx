import { cn } from "cn";
import { FitAddon, init as initGhostty, Terminal } from "ghostty-web";
import {
	useCallback,
	useEffect,
	useEffectEvent,
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
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import { toast } from "#/components/Toaster/toast";
import { useClipboard } from "#/hooks/useClipboard";
import { isMac } from "#/utils/platform";
import { terminalWebsocketUrl } from "#/utils/terminal";
import { createLinkProvider } from "./linkProvider";
import { Osc52Parser } from "./osc52";
import type { ConnectionStatus } from "./types";

export type WorkspaceTerminalHandle = {
	refit: () => void;
};

type WorkspaceTerminalProps = {
	ref?: React.Ref<WorkspaceTerminalHandle>;
	agentId: string | undefined;
	className?: string;
	autoFocus?: boolean;
	isVisible?: boolean;
	initialCommand?: string;
	containerName?: string;
	containerUser?: string;
	onStatusChange?: (status: ConnectionStatus) => void;
	onError?: (error: Error) => void;
	onContentReady?: () => void;
	reconnectionToken: string;
	/**
	 * The session ID correlates all logs, requests, and telemetry for this
	 * terminal session. Unlike the reconnection token, it is not persisted in
	 * the URL: a new page load (including a reload) is a new session.
	 */
	sessionId: string;
	baseUrl?: string;
	terminalFontFamily?: string;
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

// The terminal emulator runs in WebAssembly, which must be loaded once
// before any terminal is created.
let ghosttyInitPromise: Promise<void> | undefined;
const loadGhostty = () => {
	ghosttyInitPromise ??= initGhostty().catch((error: unknown) => {
		ghosttyInitPromise = undefined;
		throw error;
	});
	return ghosttyInitPromise;
};

export const WorkspaceTerminal = ({
	ref,
	agentId,
	className,
	autoFocus = true,
	isVisible = true,
	initialCommand,
	containerName,
	containerUser,
	onStatusChange,
	onError,
	onContentReady,
	reconnectionToken,
	sessionId,
	baseUrl,
	terminalFontFamily = DEFAULT_TERMINAL_FONT_FAMILY,
	backgroundColor,
	onOpenLink,
	loading = false,
	errorMessage,
	testId,
}: WorkspaceTerminalProps) => {
	const terminalWrapperRef = useRef<HTMLDivElement>(null);
	const terminalRef = useRef<Terminal | undefined>(undefined);
	const fitAddonRef = useRef<FitAddon | undefined>(undefined);
	const websocketRef = useRef<Websocket | undefined>(undefined);
	const firstWriteTerminalRef = useRef<Terminal | undefined>(undefined);
	const handleOpenLink = useEffectEvent((uri: string) => {
		if (onOpenLink) {
			onOpenLink(uri);
		} else {
			window.open(uri, "_blank", "noopener");
		}
	});
	const handleStatusChange = useEffectEvent((status: ConnectionStatus) => {
		onStatusChange?.(status);
	});
	const [terminal, setTerminal] = useState<Terminal>();
	const { copyToClipboard, readFromClipboard } = useClipboard();

	const [hasSelection, setHasSelection] = useState(false);
	const handleContextMenuOpenChange = (open: boolean) => {
		if (open) {
			setHasSelection(Boolean(terminal?.hasSelection()));
		}
	};
	const copyTerminalSelection = () => {
		const selection = terminal?.getSelection();
		if (selection) {
			void copyToClipboard(selection);
		}
	};
	const pasteIntoTerminal = async () => {
		if (!terminal) {
			return;
		}
		try {
			const text = await readFromClipboard();
			if (text) {
				terminal.paste(text);
			}
		} catch (error) {
			toast.error("Failed to paste from clipboard");
			console.error(error);
		} finally {
			terminal.focus();
		}
	};

	const reportTerminalError = useEffectEvent((error: Error) => {
		console.error(error);
		onError?.(error);
	});

	// Notify after the first output paints so consumers can hide connection
	// latency. The write callback runs on the animation frame after the
	// terminal has rendered the written data.
	const writeToTerminal = useEffectEvent(
		(target: Terminal, data: string | Uint8Array) => {
			if (firstWriteTerminalRef.current === target) {
				target.write(data);
				return;
			}
			firstWriteTerminalRef.current = target;
			target.write(data, () => {
				onContentReady?.();
			});
		},
	);

	const copyOsc52Text = useEffectEvent((text: string) => {
		void copyToClipboard(text);
	});

	const getTerminalDimensions = useEffectEvent(
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
	);

	const refit = useCallback(() => {
		const fitAddon = fitAddonRef.current;
		const currentTerminal = terminalRef.current;
		if (!fitAddon || !currentTerminal) {
			return;
		}

		// Fitting a zero-size container clamps the terminal and PTY to the minimum column count.
		const mountNode = terminalWrapperRef.current;
		if (
			!mountNode ||
			mountNode.clientWidth === 0 ||
			mountNode.clientHeight === 0
		) {
			return;
		}

		// FitAddon.fit() ignores calls made within 50ms of a previous resize,
		// which drops resizes that happen in quick succession. Resizing from
		// the proposed dimensions directly keeps every refit.
		const dimensions = fitAddon.proposeDimensions();
		if (
			!dimensions ||
			(dimensions.cols === currentTerminal.cols &&
				dimensions.rows === currentTerminal.rows)
		) {
			return;
		}
		currentTerminal.resize(dimensions.cols, dimensions.rows);
	}, []);

	useImperativeHandle(
		ref,
		() => ({
			refit,
		}),
		[refit],
	);

	useEffect(() => {
		if (!isVisible) {
			return;
		}

		const mountNode = terminalWrapperRef.current;
		if (!mountNode) {
			reportTerminalError(new Error("Terminal mount container is unavailable"));
			return;
		}

		let disposed = false;
		let teardown: (() => void) | undefined;

		const setupTerminal = () => {
			const nextTerminal = new Terminal({
				allowTransparency: true,
				disableStdin: false,
				fontFamily: terminalFontFamily,
				fontSize: 16,
				...(backgroundColor ? { theme: { background: backgroundColor } } : {}),
			});

			const fitAddon = new FitAddon();
			nextTerminal.loadAddon(fitAddon);

			const copySelection = () => {
				const selection = nextTerminal.getSelection();
				if (selection) {
					copyToClipboard(selection);
				}
			};

			// There is no way to remove this handler, so we must attach it once
			// and rely on a ref to send it to the current socket. The handler only
			// receives keydown events; returning true marks the event as handled.
			nextTerminal.attachCustomKeyEventHandler((event) => {
				// Make shift+enter send ^[^M (escaped carriage return). Applications
				// typically take this to mean to insert a literal newline.
				if (event.shiftKey && event.key === "Enter") {
					websocketRef.current?.send(
						encodeTerminalPayload({ data: ESCAPED_CARRIAGE_RETURN }),
					);
					return true;
				}

				// ghostty-web leaves ctrl+v to the browser, which pastes. Send ^V like
				// other terminals do, so programs such as vim receive it. Pasting
				// uses ctrl+shift+v or the context menu instead.
				if (
					event.ctrlKey &&
					!event.shiftKey &&
					!event.altKey &&
					!event.metaKey &&
					event.code === "KeyV"
				) {
					websocketRef.current?.send(encodeTerminalPayload({ data: "\x16" }));
					return true;
				}

				// Make ctrl+shift+c (command+shift+c on macOS) copy the selected
				// text. By default this usually launches the browser dev tools, but
				// users expect this keybinding to copy when in the context of the
				// web terminal.
				if (
					(isMac() ? event.metaKey : event.ctrlKey) &&
					event.shiftKey &&
					event.key === "C"
				) {
					copySelection();
					return true;
				}

				return false;
			});

			// Browsers don't support automatic copy to the X11 primary
			// selection (highlighted text that can be pasted with
			// middle-click). Instead, copy-on-select writes to the
			// system clipboard. This means users can't middle-click
			// paste in the terminal after selecting, but this tradeoff
			// is necessary because web browsers don't expose primary
			// selection APIs. Most web terminal users paste with
			// Ctrl+Shift+V, Cmd+V on macOS, or the context menu anyway.
			const selectionListener = nextTerminal.onSelectionChange(() => {
				copySelection();
			});

			// open() focuses the terminal. Skip that so autoFocus stays in control
			// and background terminals do not take focus when they mount.
			const focus = nextTerminal.focus;
			nextTerminal.focus = () => {};
			try {
				nextTerminal.open(mountNode);
			} finally {
				nextTerminal.focus = focus;
			}
			nextTerminal.registerLinkProvider(
				createLinkProvider(nextTerminal, handleOpenLink),
			);

			terminalRef.current = nextTerminal;
			fitAddonRef.current = fitAddon;
			refit();

			window.addEventListener("resize", refit);

			const resizeObserver = new ResizeObserver(() => {
				refit();
			});
			resizeObserver.observe(mountNode);

			setTerminal(nextTerminal);

			teardown = () => {
				window.removeEventListener("resize", refit);
				resizeObserver.disconnect();
				selectionListener.dispose();
				terminalRef.current = undefined;
				fitAddonRef.current = undefined;
				setTerminal(undefined);
				nextTerminal.dispose();
			};
		};

		loadGhostty()
			.then(() => {
				if (disposed) {
					return;
				}
				setupTerminal();
			})
			.catch((error: unknown) => {
				if (disposed) {
					return;
				}
				reportTerminalError(
					error instanceof Error ? error : new Error(String(error)),
				);
				handleStatusChange("disconnected");
			});

		return () => {
			disposed = true;
			teardown?.();
		};
	}, [isVisible, copyToClipboard, refit, terminalFontFamily, backgroundColor]);

	useEffect(() => {
		if (!isVisible) {
			return;
		}

		refit();
	}, [isVisible, refit]);

	useEffect(() => {
		if (!terminal || !isVisible || !autoFocus || loading) {
			return;
		}

		const frame = requestAnimationFrame(() => {
			terminal.focus();
		});

		return () => {
			cancelAnimationFrame(frame);
		};
	}, [terminal, isVisible, autoFocus, loading]);

	useEffect(() => {
		if (!terminal || !isVisible) {
			return;
		}

		terminal.clear();
		terminal.options.disableStdin = true;

		if (loading) {
			return;
		}

		if (errorMessage) {
			writeToTerminal(terminal, `${errorMessage}\r\n`);
			handleStatusChange("disconnected");
			return;
		}

		if (!agentId) {
			const error = new Error("Terminal requires agentId to connect");
			reportTerminalError(error);
			writeToTerminal(terminal, `${error.message}\r\n`);
			handleStatusChange("disconnected");
			return;
		}

		refit();
		// Fall back to standard dimensions if the terminal has no size yet.
		// The correct size will be sent once the ResizeObserver fires.
		const initialDimensions = getTerminalDimensions(terminal) ?? {
			height: 24,
			width: 80,
		};

		const osc52Parser = new Osc52Parser();

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
			sessionId,
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
					terminal.options.disableStdin = false;
					refit();
					scheduleTerminalResize();
					handleStatusChange("connected");
				});
				websocket.addEventListener(WebsocketEvent.error, (_, event) => {
					if (disposed) {
						return;
					}
					console.error("WebSocket error:", {
						client_session_id: sessionId,
						event,
					});
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
					// String data exclusively occurs when testing.
					// "jest-websocket-mock" doesn't support ArrayBuffer.
					const data =
						typeof event.data === "string"
							? event.data
							: new Uint8Array(event.data);
					for (const text of osc52Parser.feed(data)) {
						copyOsc52Text(text);
					}
					writeToTerminal(terminal, data);
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
				console.error("WebSocket connection failed:", {
					client_session_id: sessionId,
					error,
				});
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
		isVisible,
		agentId,
		baseUrl,
		containerName,
		containerUser,
		errorMessage,
		initialCommand,
		loading,
		reconnectionToken,
		sessionId,
		refit,
		terminal,
	]);

	return (
		<ContextMenu onOpenChange={handleContextMenuOpenChange}>
			<ContextMenuTrigger asChild disabled={isMac()}>
				<div
					className={cn(
						"workspace-terminal h-full w-full flex-1 min-h-0 overflow-hidden bg-surface-tertiary p-1 caret-transparent",
						className,
					)}
					ref={terminalWrapperRef}
					// The terminal's hidden textarea is the tab stop. Without this,
					// the terminal makes the wrapper a second one.
					tabIndex={-1}
					data-testid={testId}
				/>
			</ContextMenuTrigger>
			<ContextMenuContent
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					terminal?.focus();
				}}
			>
				<ContextMenuItem
					disabled={!hasSelection}
					onSelect={copyTerminalSelection}
				>
					Copy
				</ContextMenuItem>
				<ContextMenuItem onSelect={() => void pasteIntoTerminal()}>
					Paste
				</ContextMenuItem>
			</ContextMenuContent>
		</ContextMenu>
	);
};
