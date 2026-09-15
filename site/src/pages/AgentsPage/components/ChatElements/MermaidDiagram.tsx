import DOMPurify from "dompurify";
import { TriangleAlertIcon } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { useIsCodeFenceIncomplete } from "streamdown";
import { getErrorMessage } from "#/api/errors";
import { Spinner } from "#/components/Spinner/Spinner";
import { useTheme } from "#/theme/context";
import { generateUUID } from "#/utils/random";

type RenderState =
	| { status: "pending" }
	| { status: "rendered"; svg: string }
	| { status: "error"; message: string };

interface MermaidDiagramProps {
	source: string;
	/** Rendered below the error message so the viewer can still read
	 * the diagram source when Mermaid rejects it. */
	fallback: ReactNode;
}

// Elements that can trigger a network request or navigation when the
// SVG is inserted into the page. Mermaid's strict security level
// already encodes HTML in labels, but shapes such as flowchart image
// nodes still emit <image href> pointing at attacker-chosen hosts,
// which would disclose the viewer's IP (same class as Cure53
// CDM-02-006 for markdown images).
const forbiddenTags = [
	"a",
	"audio",
	"embed",
	"iframe",
	"image",
	"img",
	"object",
	"script",
	"use",
	"video",
];

const renderDiagram = async (
	source: string,
	isDark: boolean,
): Promise<string> => {
	const { default: mermaid } = await import("mermaid");
	mermaid.initialize({
		startOnLoad: false,
		securityLevel: "strict",
		suppressErrorRendering: true,
		theme: isDark ? "dark" : "neutral",
		// Emitted as font-family in the SVG's own stylesheet, so the
		// diagram picks up the app font from its container.
		fontFamily: "inherit",
		htmlLabels: false,
		flowchart: { htmlLabels: false },
	});
	const { svg } = await mermaid.render(`mermaid-${generateUUID()}`, source);
	return DOMPurify.sanitize(svg, {
		USE_PROFILES: { html: true, svg: true, svgFilters: true },
		FORBID_TAGS: forbiddenTags,
	});
};

/**
 * Renders a fenced ```mermaid block from chat markdown as an SVG
 * diagram once the fence is closed.
 */
export const MermaidDiagram = ({ source, fallback }: MermaidDiagramProps) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const isIncomplete = useIsCodeFenceIncomplete();
	const [state, setState] = useState<RenderState>({ status: "pending" });

	useEffect(() => {
		if (isIncomplete) {
			return;
		}
		// The previous SVG stays on screen until the new one is ready so a
		// theme switch or a late source edit does not flash the spinner.
		let cancelled = false;
		renderDiagram(source, isDark).then(
			(svg) => {
				if (!cancelled) {
					setState({ status: "rendered", svg });
				}
			},
			(error: unknown) => {
				if (!cancelled) {
					setState({
						status: "error",
						message: getErrorMessage(error, "Unknown error"),
					});
				}
			},
		);
		return () => {
			cancelled = true;
		};
	}, [source, isDark, isIncomplete]);

	if (state.status === "error") {
		return (
			<div className="my-4 overflow-hidden rounded-md border border-solid border-border-default">
				<div
					role="alert"
					className="flex items-start gap-2 border-0 border-b border-solid border-border-default bg-surface-orange px-3 py-2 text-xs text-content-warning"
				>
					<TriangleAlertIcon aria-hidden className="mt-0.5 size-3.5 shrink-0" />
					<div className="min-w-0">
						<p className="m-0 font-medium">Could not render Mermaid diagram</p>
						<pre className="m-0 mt-1 whitespace-pre-wrap break-words font-mono text-[11px] leading-snug opacity-90">
							{state.message.trim()}
						</pre>
					</div>
				</div>
				<div className="[&>*]:my-0 [&>*]:rounded-none [&>*]:border-0">
					{fallback}
				</div>
			</div>
		);
	}

	if (state.status === "pending") {
		return (
			<div
				role="status"
				className="my-4 flex min-h-40 items-center justify-center gap-2 rounded-md border border-solid border-border-default bg-surface-primary text-xs text-content-secondary"
			>
				<Spinner loading size="sm" />
				Rendering diagram
			</div>
		);
	}

	return (
		<div
			className="my-4 overflow-x-auto rounded-md border border-solid border-border-default bg-surface-primary p-4 [&>svg]:mx-auto [&>svg]:block [&>svg]:h-auto [&>svg]:max-w-full"
			// Output is sanitized by sanitizeDiagramSvg before it reaches
			// React, so the only HTML here is Mermaid's own SVG markup.
			dangerouslySetInnerHTML={{ __html: state.svg }}
		/>
	);
};
