import { RotateCwIcon } from "lucide-react";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { getChatArtifactFrameURL } from "#/api/api";
import { Button } from "#/components/Button/Button";
import { useTheme } from "#/theme/context";
import { acceptArtifactMessage } from "./artifactBridge";

type ArtifactFrameProps = {
	artifactId: string;
	version: number;
	title: string;
	/** Receives accepted prompts. Without it, the frame posts to nobody. */
	onPrompt?: (text: string) => void;
};

/**
 * Renders an HTML artifact from the frame route. The route sends a CSP
 * sandbox header, and the iframe sandbox omits allow-same-origin, so the
 * page gets an opaque origin with no cookie and no API access. Never use
 * srcdoc here: a srcdoc frame inherits the SPA CSP, which blocks scripts.
 */
export const ArtifactFrame: React.FC<ArtifactFrameProps> = (props) => {
	const [reloadCount, setReloadCount] = useState(0);
	// A page that uses the Coder Kit follows the dashboard theme. A theme
	// change gives the frame a new src, so the page reloads in that theme.
	const src = getChatArtifactFrameURL(
		props.artifactId,
		props.version,
		useTheme().palette.mode,
	);
	return (
		<FrameInstance
			// A new src or a reload starts a fresh load count.
			key={`${src}#${reloadCount}`}
			src={src}
			title={props.title}
			onPrompt={props.onPrompt}
			onReload={() => setReloadCount((count) => count + 1)}
		/>
	);
};

type FrameInstanceProps = {
	src: string;
	title: string;
	onPrompt?: (text: string) => void;
	onReload: () => void;
};

const FrameInstance: React.FC<FrameInstanceProps> = ({
	src,
	title,
	onPrompt,
	onReload,
}) => {
	const frameRef = useRef<HTMLIFrameElement>(null);
	const loadCountRef = useRef(0);
	const [navigated, setNavigated] = useState(false);
	const hasPromptHandler = onPrompt !== undefined;

	const handleMessage = useEffectEvent((event: MessageEvent) => {
		const text = acceptArtifactMessage(event, {
			frame: frameRef.current,
			navigated,
			activeElement: document.activeElement,
			userActivationActive: navigator.userActivation?.isActive ?? false,
		});
		if (text !== undefined) {
			onPrompt?.(text);
		}
	});

	useEffect(() => {
		if (!hasPromptHandler) {
			return;
		}
		window.addEventListener("message", handleMessage);
		return () => window.removeEventListener("message", handleMessage);
	}, [hasPromptHandler]);

	return (
		<div className="relative flex h-full min-h-0 flex-1 flex-col">
			{navigated && (
				<div
					role="status"
					className="flex shrink-0 items-center justify-between gap-2 border-0 border-b border-solid border-border bg-surface-secondary px-3 py-1.5 text-xs text-content-secondary"
				>
					The page navigated away. Its messages are ignored until you reload it.
					<Button size="sm" variant="outline" onClick={onReload}>
						<RotateCwIcon />
						Reload
					</Button>
				</div>
			)}
			<iframe
				ref={frameRef}
				title={title}
				src={src}
				sandbox="allow-scripts"
				referrerPolicy="no-referrer"
				className="min-h-0 w-full flex-1 border-0 bg-white"
				onLoad={() => {
					loadCountRef.current += 1;
					if (loadCountRef.current > 1) {
						setNavigated(true);
					}
				}}
			/>
		</div>
	);
};
