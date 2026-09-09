import {
	type ReactNode,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useTheme } from "#/theme/context";
import { connectMCPApp } from "./bridge";

interface MCPAppFrameProps {
	src: string;
	title: string;
	args: unknown;
	result: unknown;
	displayMode: "inline" | "fullscreen";
	fallback: ReactNode;
}

export const MCPAppFrame = ({
	src,
	title,
	args,
	result,
	displayMode,
	fallback,
}: MCPAppFrameProps) => {
	const frameRef = useRef<HTMLIFrameElement>(null);
	const connectionRef = useRef<ReturnType<typeof connectMCPApp>>(null);
	const loadCountRef = useRef(0);
	const { buildInfo } = useDashboard();
	const theme = useTheme();
	const themeMode = theme.palette.mode;
	const [status, setStatus] = useState<"loading" | "ready" | "error">(
		"loading",
	);
	const [size, setSize] = useState<{ height: number; width?: number }>({
		height: 320,
	});
	const failed = status === "error";
	const getToolData = useEffectEvent(() => ({ args, result }));
	const getContext = useEffectEvent(() => ({
		theme: themeMode,
		displayMode,
		containerDimensions: {
			width: frameRef.current?.clientWidth ?? 0,
			height: frameRef.current?.clientHeight ?? 0,
		},
		platform: "web" as const,
		userAgent: navigator.userAgent,
	}));
	useEffect(() => {
		const frame = frameRef.current;
		if (!frame || failed) return;
		let animationFrame = 0;
		let pendingSize: { height?: number; width?: number } = {};
		const onError = () => setStatus("error");
		// A sandboxed document can still navigate its own frame, and the
		// replacement keeps the same opaque origin and window, so any load
		// after the served document ends the app instead of trusting it.
		const onLoad = () => {
			loadCountRef.current += 1;
			if (loadCountRef.current > 1) onError();
		};
		frame.addEventListener("error", onError);
		frame.addEventListener("load", onLoad);
		const timeout = window.setTimeout(onError, 15000);
		const connection = connectMCPApp({
			frame,
			hostVersion: buildInfo.version,
			getToolData,
			getContext,
			onReady: () => {
				window.clearTimeout(timeout);
				setStatus("ready");
			},
			onSizeChange: (nextSize) => {
				if (displayMode !== "inline") return;
				pendingSize = { ...pendingSize, ...nextSize };
				if (animationFrame) return;
				animationFrame = window.requestAnimationFrame(() => {
					animationFrame = 0;
					const requestedSize = pendingSize;
					pendingSize = {};
					setSize((current) => ({ ...current, ...requestedSize }));
				});
			},
		});
		connectionRef.current = connection;
		return () => {
			connectionRef.current = null;
			connection.disconnect();
			frame.removeEventListener("error", onError);
			frame.removeEventListener("load", onLoad);
			window.clearTimeout(timeout);
			window.cancelAnimationFrame(animationFrame);
		};
	}, [buildInfo.version, displayMode, failed]);
	useEffect(() => {
		connectionRef.current?.notifyHostContextChanged({ theme: themeMode });
	}, [themeMode]);
	if (status === "error")
		return (
			<div className="h-full overflow-auto">
				<p role="alert" className="p-3 text-sm text-content-secondary">
					This app could not be displayed. The tool output is shown below.
				</p>
				{fallback}
			</div>
		);
	return (
		<div
			className="relative w-full"
			style={{
				height: displayMode === "inline" ? size.height : "100%",
				width: displayMode === "inline" ? size.width : undefined,
				maxWidth: "100%",
			}}
		>
			{status === "loading" && (
				<div
					role="status"
					className="absolute inset-0 flex items-center justify-center gap-2 bg-surface-primary"
				>
					<Spinner size="sm" loading />
					Loading app...
				</div>
			)}
			<iframe
				ref={frameRef}
				src={src}
				title={title}
				sandbox="allow-scripts"
				referrerPolicy="no-referrer"
				className="h-full w-full border-0"
				style={{ visibility: status === "ready" ? undefined : "hidden" }}
			/>
		</div>
	);
};
