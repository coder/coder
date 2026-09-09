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
	const { buildInfo } = useDashboard();
	const theme = useTheme();
	const [status, setStatus] = useState<"loading" | "ready" | "error">(
		"loading",
	);
	const [size, setSize] = useState<{ height: number; width?: number }>({
		height: 320,
	});
	const failed = status === "error";
	const getToolData = useEffectEvent(() => ({ args, result }));
	const getContext = useEffectEvent(() => ({
		theme: theme.palette.mode,
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
		frame.addEventListener("error", onError);
		const timeout = window.setTimeout(onError, 15000);
		const disconnect = connectMCPApp({
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
		return () => {
			disconnect();
			frame.removeEventListener("error", onError);
			window.clearTimeout(timeout);
			window.cancelAnimationFrame(animationFrame);
		};
	}, [buildInfo.version, displayMode, failed]);
	if (status === "error")
		return (
			<div className="h-full overflow-auto">
				<p role="alert" className="p-3 text-sm text-content-secondary">
					This app could not initialize. The tool output is shown below.
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
				style={{ visibility: status === "ready" ? "visible" : "hidden" }}
			/>
		</div>
	);
};
