import { useEffect, useRef } from "react";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { pageTitle } from "#/utils/page";
import { BrowserToolbar } from "./BrowserToolbar";
import {
	type BrowserClientMessage,
	keyMessage,
	modifierBits,
	mouseButton,
	toVideoPoint,
} from "./protocol";
import { useBrowserStream } from "./useBrowserStream";

const DOUBLE_CLICK_MS = 500;
const DOUBLE_CLICK_DISTANCE = 5;

type BrowserViewProps = {
	streamUrl: string;
	workspaceName: string;
};

export const BrowserView: React.FC<BrowserViewProps> = ({
	streamUrl,
	workspaceName,
}) => {
	const { canvasRef, status, navState, closeReason, send, reconnect } =
		useBrowserStream(streamUrl);
	const addressRef = useRef<HTMLInputElement>(null);
	const surfaceRef = useRef<HTMLDivElement>(null);
	const pendingMove = useRef<BrowserClientMessage | null>(null);
	const lastDown = useRef({ time: 0, x: 0, y: 0, button: -1, count: 0 });

	// React registers wheel listeners as passive, so preventing page
	// scroll needs a native listener.
	useEffect(() => {
		const canvas = canvasRef.current;
		if (!canvas) {
			return;
		}
		const onWheel = (event: WheelEvent) => {
			event.preventDefault();
			const point = toVideoPoint(
				event.clientX,
				event.clientY,
				canvas.getBoundingClientRect(),
				canvas.width,
				canvas.height,
			);
			send({
				type: "mouse",
				action: "wheel",
				...point,
				delta_x: event.deltaX,
				delta_y: event.deltaY,
				modifiers: modifierBits(event),
			});
		};
		canvas.addEventListener("wheel", onWheel, { passive: false });
		return () => canvas.removeEventListener("wheel", onWheel);
	}, [canvasRef, send]);

	const pointFor = (event: React.PointerEvent) => {
		const canvas = canvasRef.current;
		if (!canvas) {
			return { x: 0, y: 0 };
		}
		return toVideoPoint(
			event.clientX,
			event.clientY,
			canvas.getBoundingClientRect(),
			canvas.width,
			canvas.height,
		);
	};

	const clickCount = (button: number, x: number, y: number) => {
		const now = performance.now();
		const prev = lastDown.current;
		const repeat =
			prev.button === button &&
			now - prev.time < DOUBLE_CLICK_MS &&
			Math.abs(prev.x - x) < DOUBLE_CLICK_DISTANCE &&
			Math.abs(prev.y - y) < DOUBLE_CLICK_DISTANCE;
		const count = repeat ? prev.count + 1 : 1;
		lastDown.current = { time: now, x, y, button, count };
		return count;
	};

	return (
		<div className="flex h-full w-full flex-col bg-surface-primary">
			<title>{pageTitle(navState?.title || "Browser", workspaceName)}</title>
			<BrowserToolbar
				navState={navState}
				status={status}
				addressRef={addressRef}
				onNavigate={(url) => {
					send({ type: "navigate", url });
					surfaceRef.current?.focus();
				}}
				onBack={() => send({ type: "back" })}
				onForward={() => send({ type: "forward" })}
				onReload={() => send({ type: "reload" })}
			/>
			<div className="relative min-h-0 flex-1 bg-surface-tertiary">
				<div
					ref={surfaceRef}
					role="application"
					aria-label="Workspace browser"
					// biome-ignore lint/a11y/noNoninteractiveTabindex: the remote page needs keyboard focus to receive key events.
					tabIndex={0}
					className="h-full w-full cursor-default outline-none"
					onContextMenu={(event) => event.preventDefault()}
					onPointerDown={(event) => {
						event.currentTarget.focus();
						event.currentTarget.setPointerCapture(event.pointerId);
						const point = pointFor(event);
						send({
							type: "mouse",
							action: "down",
							...point,
							button: mouseButton(event.button),
							buttons: event.buttons,
							click_count: clickCount(event.button, point.x, point.y),
							modifiers: modifierBits(event),
						});
					}}
					onPointerUp={(event) => {
						const point = pointFor(event);
						send({
							type: "mouse",
							action: "up",
							...point,
							button: mouseButton(event.button),
							buttons: event.buttons,
							click_count: lastDown.current.count,
							modifiers: modifierBits(event),
						});
					}}
					onPointerMove={(event) => {
						const scheduled = pendingMove.current !== null;
						pendingMove.current = {
							type: "mouse",
							action: "move",
							...pointFor(event),
							buttons: event.buttons,
							modifiers: modifierBits(event),
						};
						if (!scheduled) {
							requestAnimationFrame(() => {
								if (pendingMove.current) {
									send(pendingMove.current);
								}
								pendingMove.current = null;
							});
						}
					}}
					onKeyDown={(event) => {
						const shortcut = event.ctrlKey || event.metaKey;
						if (shortcut && event.key === "v") {
							// Let the browser fire a paste event with the clipboard.
							return;
						}
						event.preventDefault();
						if (shortcut && event.key === "l") {
							addressRef.current?.focus();
							return;
						}
						send(keyMessage(event, "down"));
					}}
					onKeyUp={(event) => {
						event.preventDefault();
						send(keyMessage(event, "up"));
					}}
					onPaste={(event) => {
						event.preventDefault();
						const text = event.clipboardData.getData("text/plain");
						if (text) {
							send({ type: "insert_text", text });
						}
					}}
				>
					<canvas
						ref={canvasRef}
						className="block h-full w-full object-contain"
					/>
				</div>
				{status === "connecting" && (
					<div className="absolute inset-0 flex items-center justify-center">
						<Loader label="Starting browser" />
					</div>
				)}
				{status === "disconnected" && (
					<div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-surface-primary text-center">
						<p className="m-0 text-content-primary">Browser disconnected</p>
						{closeReason && (
							<p className="m-0 max-w-lg text-sm text-content-secondary">
								{closeReason}
							</p>
						)}
						<Button onClick={reconnect}>Reconnect</Button>
					</div>
				)}
			</div>
		</div>
	);
};
