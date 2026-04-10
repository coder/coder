import { useTheme } from "@emotion/react";
import {
	File as FileViewer,
	type SupportedLanguages,
} from "@pierre/diffs/react";
import type { ComponentPropsWithRef, ReactNode } from "react";
import {
	type Components,
	defaultRehypePlugins,
	Streamdown,
	type UrlTransform,
} from "streamdown";
import { cn } from "#/utils/cn";

interface ResponseProps extends Omit<ComponentPropsWithRef<"div">, "children"> {
	children: string;
	urlTransform?: UrlTransform;
	/** Enable streaming-mode Streamdown with incomplete-markdown
	 * preprocessing (remend) and useTransition-based render
	 * scheduling. Pass true only for live-streaming output. */
	streaming?: boolean;
}

// Omit rehype-raw so HTML-like syntax in LLM output is rendered as
// escaped text instead of being parsed by the HTML5 engine. Without
// this, JSX fragments such as <ComponentName prop={value} /> are
// consumed by rehype-raw and then stripped by rehype-sanitize,
// silently destroying content mid-stream.
const chatRehypePlugins = [
	defaultRehypePlugins.sanitize,
	defaultRehypePlugins.harden,
];

const fileViewerCSS =
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; }";

const fileViewerTheme = {
	light: "github-light",
	dark: "github-dark-high-contrast",
} as const;

type HastNode = {
	type?: string;
	value?: string;
	children?: HastNode[];
	tagName?: string;
	properties?: {
		className?: string[] | string;
	};
};

type MarkdownComponentProps = {
	href?: string;
	children?: ReactNode;
	node?: HastNode;
	type?: string;
	checked?: boolean;
	disabled?: boolean;
	className?: string;
};

type FileViewerThemeType = "light" | "dark";

/**
 * Recursively extracts text from a HAST node tree. This is plain
 * data (not React elements), so it's reliable to traverse.
 */
const getHastText = (node: HastNode | null | undefined): string => {
	if (!node) {
		return "";
	}
	if (node.type === "text") return node.value ?? "";
	if (node.children) return node.children.map(getHastText).join("");
	return "";
};

const getClassNames = (className: string[] | string | undefined): string[] => {
	if (typeof className === "string") {
		return className.split(/\s+/).filter(Boolean);
	}
	if (!Array.isArray(className)) {
		return [];
	}
	return className.filter(
		(classToken): classToken is string => typeof classToken === "string",
	);
};

const createComponents = (
	fileViewerThemeType: FileViewerThemeType,
	viewerTheme: (typeof fileViewerTheme)[FileViewerThemeType],
): Components => {
	return {
		a: ({ href, children }: MarkdownComponentProps) => (
			<a
				href={href}
				target="_blank"
				rel="noopener noreferrer"
				className="text-content-link no-underline hover:underline hover:decoration-content-link"
			>
				{children}
			</a>
		),
		// Headings scaled for a 13px base using a tight,
		// Apple-like progression.
		h1: ({ children }: MarkdownComponentProps) => (
			<h1 className="mb-3 mt-5 text-xl font-semibold leading-snug first:mt-0">
				{children}
			</h1>
		),
		h2: ({ children }: MarkdownComponentProps) => (
			<h2 className="mb-2 mt-4 text-base font-semibold leading-snug first:mt-0">
				{children}
			</h2>
		),
		h3: ({ children }: MarkdownComponentProps) => (
			<h3 className="mb-1.5 mt-3 text-[15px] font-semibold leading-snug first:mt-0">
				{children}
			</h3>
		),
		h4: ({ children }: MarkdownComponentProps) => (
			<h4 className="mb-1 mt-3 text-sm font-semibold leading-snug first:mt-0">
				{children}
			</h4>
		),
		h5: ({ children }: MarkdownComponentProps) => (
			<h5 className="mb-1 mt-2 text-[13px] font-semibold leading-snug first:mt-0">
				{children}
			</h5>
		),
		h6: ({ children }: MarkdownComponentProps) => (
			<h6 className="mb-1 mt-2 text-xs font-semibold leading-snug text-content-secondary first:mt-0">
				{children}
			</h6>
		),
		// GFM task-list checkboxes: render a styled replacement
		// for the native <input type="checkbox" disabled> element.
		input: ({ type, checked, disabled }: MarkdownComponentProps) => {
			if (type !== "checkbox") {
				return <input type={type} disabled={disabled} />;
			}
			return (
				<span
					aria-hidden="true"
					className={cn(
						"mr-2 inline-flex size-4 shrink-0 items-center justify-center",
						"rounded-sm border border-solid",
						"align-middle relative -top-px",
						checked
							? "border-content-link bg-content-link text-white"
							: "border-border-default bg-surface-primary",
					)}
				>
					{checked && (
						<svg
							className="size-3"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
							strokeWidth={3}
						>
							<path
								strokeLinecap="round"
								strokeLinejoin="round"
								d="M5 13l4 4L19 7"
							/>
						</svg>
					)}
				</span>
			);
		},

		// Horizontal rule: reset browser default inset/ridge border
		// (preflight is disabled) to a clean 1px solid line.
		hr: () => (
			<hr className="my-6 border-0 border-t border-solid border-border-default" />
		),
		// Table cells: streamdown defaults to text-sm (14px).
		// Drop the explicit size so cells inherit the 13px base.
		th: ({ children }: MarkdownComponentProps) => (
			<th className="whitespace-nowrap px-4 py-2 text-left font-semibold">
				{children}
			</th>
		),
		td: ({ children }: MarkdownComponentProps) => (
			<td className="px-4 py-2">{children}</td>
		),
		// Inline code only — fenced blocks are handled by the pre override.
		code: ({ children }: MarkdownComponentProps) => (
			<code className="rounded bg-surface-quaternary/25 px-1 py-0.5 font-mono text-content-primary">
				{children}
			</code>
		),
		// Fenced code blocks: extract language and content from the HAST
		// node directly (plain data), then render with FileViewer.
		pre: ({ node }: MarkdownComponentProps) => {
			const codeChild = node?.children?.[0];
			if (codeChild?.tagName === "code") {
				const classes = getClassNames(codeChild.properties?.className);
				const langClass = classes.find((c: string) =>
					c.startsWith("language-"),
				);
				const lang = langClass ? langClass.replace("language-", "") : "text";
				const content = getHastText(codeChild).trimEnd();
				if (content) {
					return (
						<div className="my-4 overflow-hidden rounded-xl border border-solid border-border-default text-2xs">
							<FileViewer
								file={{
									name: `block.${lang}`,
									lang: lang as SupportedLanguages,
									contents: content,
									cacheKey: content,
								}}
								options={{
									overflow: "scroll",
									themeType: fileViewerThemeType,
									disableFileHeader: true,
									disableLineNumbers: true,
									theme: viewerTheme,
									unsafeCSS: fileViewerCSS,
								}}
							/>
						</div>
					);
				}
			}
			return <pre>{node?.children?.map?.(() => null)}</pre>;
		},
	};
};

// Precompute component maps for both themes at module scope so
// every Response instance shares the same stable references.
// This prevents Streamdown from discarding its cached render
// tree on each parent re-render.
const componentsByTheme: Record<FileViewerThemeType, Components> = {
	light: createComponents("light", fileViewerTheme.light),
	dark: createComponents("dark", fileViewerTheme.dark),
};

export const Response = ({
	className,
	children,
	ref,
	urlTransform,
	streaming,
	...props
}: ResponseProps) => {
	const theme = useTheme();
	const fileViewerThemeType: FileViewerThemeType =
		theme.palette.mode === "dark" ? "dark" : "light";
	const components = componentsByTheme[fileViewerThemeType];

	return (
		<div
			ref={ref}
			className={cn(
				"text-[13px] leading-relaxed text-content-primary",
				className,
			)}
			{...props}
		>
			<Streamdown
				controls={false}
				components={components}
				urlTransform={urlTransform}
				rehypePlugins={chatRehypePlugins}
				mode={streaming ? "streaming" : "static"}
				parseIncompleteMarkdown={streaming}
			>
				{children}
			</Streamdown>
		</div>
	);
};
