import DOMPurify from "dompurify";
import { TriangleAlertIcon } from "lucide-react";
import type { MermaidConfig } from "mermaid";
import type { WheelEvent } from "react";
import {
	type ControlsConfig,
	type DiagramPlugin,
	type MermaidErrorComponentProps,
	type MermaidOptions,
	type PluginConfig,
	Streamdown,
	useIsCodeFenceIncomplete,
} from "streamdown";
import { Spinner } from "#/components/Spinner/Spinner";
import { useTheme } from "#/theme/context";

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

export const sanitizeDiagramSvg = (svg: string): string =>
	DOMPurify.sanitize(svg, {
		USE_PROFILES: { html: true, svg: true, svgFilters: true },
		FORBID_TAGS: forbiddenTags,
	});

// Settings that chat content must not be able to override, applied on
// top of the theme config Streamdown passes back in.
const hardenedConfig: MermaidConfig = {
	startOnLoad: false,
	securityLevel: "strict",
	suppressErrorRendering: true,
	htmlLabels: false,
	flowchart: { htmlLabels: false },
};

const fontFamily = '"Geist Variable", system-ui, sans-serif';

const themedConfigs = {
	dark: { theme: "dark", fontFamily },
	light: { theme: "neutral", fontFamily },
} satisfies Record<string, MermaidConfig>;

// Streamdown hands the config object back untyped, so recover the
// concrete one by identity instead of trusting its shape.
const resolveConfig = (config: unknown): MermaidConfig =>
	config === themedConfigs.light ? themedConfigs.light : themedConfigs.dark;

/**
 * Streamdown diagram plugin backed by a lazily imported mermaid, so the
 * library only loads for chats that actually contain a diagram. Output
 * is sanitized before Streamdown injects it into the DOM.
 */
const mermaidPlugin: DiagramPlugin = {
	name: "mermaid",
	type: "diagram",
	language: "mermaid",
	getMermaid: (config) => ({
		initialize: () => {},
		render: async (id, source) => {
			const { default: mermaid } = await import("mermaid");
			mermaid.initialize({ ...resolveConfig(config), ...hardenedConfig });
			const { svg } = await mermaid.render(id, source);
			return { svg: sanitizeDiagramSvg(svg) };
		},
	}),
};

const plugins: PluginConfig = { mermaid: mermaidPlugin };

const controls: ControlsConfig = {
	table: false,
	code: false,
	image: false,
	mermaid: { copy: true, fullscreen: true, panZoom: true, download: false },
};

const MermaidError = ({ chart, error }: MermaidErrorComponentProps) => (
	<div className="overflow-hidden rounded-md border border-solid border-border-default">
		<div
			role="alert"
			className="flex items-start gap-2 border-0 border-b border-solid border-border-default bg-surface-orange px-3 py-2 text-xs text-content-warning"
		>
			<TriangleAlertIcon aria-hidden className="mt-0.5 size-3.5 shrink-0" />
			<div className="min-w-0">
				<p className="m-0 font-medium">Could not render Mermaid diagram</p>
				<pre className="m-0 mt-1 whitespace-pre-wrap break-words font-mono text-[11px] leading-snug opacity-90">
					{error.trim()}
				</pre>
			</div>
		</div>
		<pre className="m-0 overflow-x-auto bg-surface-primary px-3 py-2 font-mono text-xs leading-5 text-content-primary">
			{chart}
		</pre>
	</div>
);

// Streamdown re-renders the diagram whenever the config object identity
// changes, so both variants are created once at module scope.
const mermaidOptionsByMode: Record<"dark" | "light", MermaidOptions> = {
	dark: { config: themedConfigs.dark, errorComponent: MermaidError },
	light: { config: themedConfigs.light, errorComponent: MermaidError },
};

// The fence must be longer than any backtick run inside the source so
// the nested markdown closes where the original block did.
export const wrapInFence = (source: string): string => {
	const longestRun = Math.max(
		0,
		...Array.from(source.matchAll(/`+/g), (match) => match[0].length),
	);
	const fence = "`".repeat(Math.max(3, longestRun + 1));
	return `${fence}mermaid\n${source}\n${fence}`;
};

// Streamdown's inline pan-zoom wrapper cancels wheel events to zoom,
// which would hijack scrolling the conversation. Stopping the capture
// phase for descendants keeps native scrolling; the fullscreen view is
// portaled outside this element and keeps wheel zoom.
const keepWheelScrolling = (event: WheelEvent<HTMLDivElement>) => {
	if (
		event.target instanceof Node &&
		event.currentTarget.contains(event.target)
	) {
		event.stopPropagation();
	}
};

/**
 * Renders a fenced ```mermaid block from chat markdown through
 * Streamdown's Mermaid block so it gets the library's copy, fullscreen,
 * and pan-zoom controls. The chat's own Streamdown instance overrides
 * fenced blocks to use the diff viewer, which bypasses that block, so
 * the source is fed to a nested instance with default components.
 * Rendering waits until the fence is closed so partially streamed
 * source never produces a flash of parse errors.
 */
export const MermaidBlock = ({ source }: { source: string }) => {
	const theme = useTheme();
	const isIncomplete = useIsCodeFenceIncomplete();
	const mermaidOptions =
		mermaidOptionsByMode[theme.palette.mode === "dark" ? "dark" : "light"];

	if (isIncomplete) {
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
			// Match the corner radius of the chat's other code blocks.
			className="my-4 [&_[data-streamdown=mermaid-block]]:rounded-md"
			onWheelCapture={keepWheelScrolling}
		>
			<Streamdown
				mode="static"
				controls={controls}
				plugins={plugins}
				mermaid={mermaidOptions}
			>
				{wrapInFence(source)}
			</Streamdown>
		</div>
	);
};
