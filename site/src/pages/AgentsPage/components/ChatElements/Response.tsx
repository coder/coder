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

const createComponents = (): Components => {
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
		pre: ({ node }: MarkdownComponentProps) => {
			const codeChild = node?.children?.[0];
			if (codeChild?.tagName === "code") {
				const classes = getClassNames(codeChild.properties?.className);
				const langClass = classes.find((c: string) =>
					c.startsWith("language-"),
				);
				const content = getHastText(codeChild).trimEnd();
				if (content) {
					return (
						<pre className="my-4 overflow-x-auto rounded-md border border-solid border-border-default bg-surface-primary px-3 py-2 font-mono text-xs leading-5 text-content-primary">
							<code className={langClass}>{content}</code>
						</pre>
					);
				}
			}
			return <pre>{getHastText(node)}</pre>;
		},
	};
};

// Precompute the component map at module scope so every Response
// instance shares the same stable references. This prevents
// Streamdown from discarding its cached render tree on each parent
// re-render.
const components = createComponents();

export const Response = ({
	className,
	children,
	ref,
	urlTransform,
	streaming,
	...props
}: ResponseProps) => {
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
