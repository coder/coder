import type { Interpolation, Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import {
	Table,
	TableBody,
	TableCell,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import isEqual from "lodash/isEqual";
import {
	type FC,
	type HTMLProps,
	type ReactElement,
	type ReactNode,
	isValidElement,
	memo,
} from "react";
import ReactMarkdown, { type Options } from "react-markdown";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { dracula } from "react-syntax-highlighter/dist/cjs/styles/prism";
import gfm from "remark-gfm";
import colors from "theme/tailwindColors";
import { cn } from "utils/cn";

interface MarkdownProps {
	/**
	 * The Markdown text to parse and render
	 */
	children: string;

	className?: string;

	/**
	 * Can override the behavior of the generated elements
	 */
	components?: Options["components"];
}

export const Markdown: FC<MarkdownProps> = (props) => {
	const { children, className, components = {} } = props;

	return (
		<ReactMarkdown
			css={markdownStyles}
			className={className}
			remarkPlugins={[gfm]}
			components={{
				a: ({ href, children }) => {
					const isExternal = href?.startsWith("http");

					return (
						<Link href={href} target={isExternal ? "_blank" : undefined}>
							{children}
						</Link>
					);
				},

				pre: ({ node, children }) => {
					if (!node || !node.children) {
						return <pre>{children}</pre>;
					}
					const firstChild = node.children[0];
					// When pre is wrapping a code, the SyntaxHighlighter is already going
					// to wrap it with a pre so we don't need it
					if (firstChild.type === "element" && firstChild.tagName === "code") {
						return <>{children}</>;
					}
					return <pre>{children}</pre>;
				},

				code: ({ node, className, children, style, ref, ...restProps }) => {
					const match = /language-(\w+)/.exec(className || "");

					return match ? (
						<SyntaxHighlighter
							style={dracula}
							language={match[1].toLowerCase() ?? "language-shell"}
							useInlineStyles={false}
							codeTagProps={{ style: {} }}
							{...restProps} // Exclude 'ref' from being passed here
						>
							{String(children)}
						</SyntaxHighlighter>
					) : (
						<code
							css={(theme) => ({
								padding: "1px 4px",
								background: theme.palette.divider,
								borderRadius: 4,
								color: theme.palette.text.primary,
								fontSize: 14,
							})}
							{...props}
						>
							{children}
						</code>
					);
				},

				table: ({ children }) => {
					return <Table>{children}</Table>;
				},

				tr: ({ children }) => {
					return <TableRow>{children}</TableRow>;
				},

				thead: ({ children }) => {
					return <TableHeader>{children}</TableHeader>;
				},

				tbody: ({ children }) => {
					return <TableBody>{children}</TableBody>;
				},

				td: ({ children }) => {
					return <TableCell>{children}</TableCell>;
				},

				th: ({ children }) => {
					return <TableCell>{children}</TableCell>;
				},

				/**
				 * 2025-02-10 - The RemarkGFM plugin that we use currently doesn't have
				 * support for special alert messages like this:
				 * ```
				 * > [!IMPORTANT]
				 * > This module will only work with Git versions >=2.34, and...
				 * ```
				 * Have to intercept all blockquotes and see if their content is
				 * formatted like an alert.
				 */
				blockquote: (parseProps) => {
					const { node: _node, children, ...renderProps } = parseProps;
					const alertContent = parseChildrenAsAlertContent(children);
					if (alertContent === null) {
						return <blockquote {...renderProps}>{children}</blockquote>;
					}

					return (
						<MarkdownGfmAlert alertType={alertContent.type} {...renderProps}>
							{alertContent.children}
						</MarkdownGfmAlert>
					);
				},

				...components,
			}}
		>
			{children}
		</ReactMarkdown>
	);
};

interface InlineMarkdownProps {
	/**
	 * The Markdown text to parse and render
	 */
	children: string;

	/**
	 * Additional element types to allow.
	 * Allows italic, bold, links, and inline code snippets by default.
	 * eg. `["ol", "ul", "li"]` to support lists.
	 */
	allowedElements?: readonly string[];

	className?: string;

	/**
	 * Can override the behavior of the generated elements
	 */
	components?: Options["components"];
}

/**
 * Supports a strict subset of Markdown that behaves well as inline/confined content.
 */
export const InlineMarkdown: FC<InlineMarkdownProps> = (props) => {
	const { children, allowedElements = [], className, components = {} } = props;

	return (
		<ReactMarkdown
			className={className}
			allowedElements={[
				"p",
				"em",
				"strong",
				"a",
				"pre",
				"code",
				...allowedElements,
			]}
			unwrapDisallowed
			components={{
				p: ({ children }) => <>{children}</>,

				a: ({ href, target, children }) => (
					<Link href={href} target={target}>
						{children}
					</Link>
				),

				code: ({ node, className, children, style, ...props }) => (
					<code
						css={(theme) => ({
							padding: "1px 4px",
							background: theme.palette.divider,
							borderRadius: 4,
							color: theme.palette.text.primary,
							fontSize: 14,
						})}
						{...props}
					>
						{children}
					</code>
				),

				...components,
			}}
		>
			{children}
		</ReactMarkdown>
	);
};

export const MemoizedMarkdown = memo(Markdown, isEqual);
export const MemoizedInlineMarkdown = memo(InlineMarkdown, isEqual);

const githubFlavoredMarkdownAlertTypes = [
	"tip",
	"note",
	"important",
	"warning",
	"caution",
];

type AlertContent = Readonly<{
	type: string;
	children: readonly ReactNode[];
}>;

function parseChildrenAsAlertContent(
	jsxChildren: ReactNode,
): AlertContent | null {
	// Have no idea why the plugin parses the data by mixing node types
	// like this. Have to do a good bit of nested filtering.
	if (!Array.isArray(jsxChildren)) {
		return null;
	}

	const mainParentNode = jsxChildren.find((node): node is ReactElement =>
		isValidElement(node),
	);
	let parentChildren = mainParentNode?.props.children;
	if (typeof parentChildren === "string") {
		// Children will only be an array if the parsed text contains other
		// content that can be turned into HTML. If there aren't any, you
		// just get one big string
		parentChildren = parentChildren.split("\n");
	}
	if (!Array.isArray(parentChildren)) {
		return null;
	}

	const outputContent = parentChildren
		.filter((el) => {
			if (isValidElement(el)) {
				return true;
			}
			return typeof el === "string" && el !== "\n";
		})
		.map((el) => {
			if (!isValidElement(el)) {
				return el;
			}
			if (el.type !== "a") {
				return el;
			}

			const recastProps = el.props as Record<string, unknown> & {
				children?: ReactNode;
			};
			if (recastProps.target === "_blank") {
				return el;
			}

			return {
				...el,
				props: {
					...recastProps,
					target: "_blank",
					children: (
						<>
							{recastProps.children}
							<span className="sr-only"> (link opens in new tab)</span>
						</>
					),
				},
			};
		});
	const [firstEl, ...remainingChildren] = outputContent;
	if (typeof firstEl !== "string") {
		return null;
	}

	const alertType = firstEl
		.trim()
		.toLowerCase()
		.replace("!", "")
		.replace("[", "")
		.replace("]", "");
	if (!githubFlavoredMarkdownAlertTypes.includes(alertType)) {
		return null;
	}

	const hasLeadingLinebreak =
		isValidElement(remainingChildren[0]) && remainingChildren[0].type === "br";
	if (hasLeadingLinebreak) {
		remainingChildren.shift();
	}

	return {
		type: alertType,
		children: remainingChildren,
	};
}

type MarkdownGfmAlertProps = Readonly<
	HTMLProps<HTMLElement> & {
		alertType: string;
	}
>;

const MarkdownGfmAlert: FC<MarkdownGfmAlertProps> = ({
	alertType,
	children,
	...delegatedProps
}) => {
	return (
		<div className="pb-6">
			<aside
				{...delegatedProps}
				className={cn(
					"border-0 border-l-4 border-solid border-border p-4 text-white",
					"[&_p]:m-0 [&_p]:mb-2",

					alertType === "important" &&
						"border-highlight-purple [&_p:first-of-type]:text-highlight-purple",

					alertType === "warning" &&
						"border-border-warning [&_p:first-of-type]:text-border-warning",

					alertType === "note" &&
						"border-highlight-sky [&_p:first-of-type]:text-highlight-sky",

					alertType === "tip" &&
						"border-highlight-green [&_p:first-of-type]:text-highlight-green",

					alertType === "caution" &&
						"border-highlight-red [&_p:first-of-type]:text-highlight-red",
				)}
			>
				<p className="font-bold">
					{alertType[0]?.toUpperCase() + alertType.slice(1).toLowerCase()}
				</p>
				{children}
			</aside>
		</div>
	);
};

const markdownStyles: Interpolation<Theme> = (theme: Theme) => ({
	fontSize: 16,
	lineHeight: "24px",

	"& h1, & h2, & h3, & h4, & h5, & h6": {
		marginTop: 32,
		marginBottom: 16,
		lineHeight: "1.25",
	},

	"& p": {
		marginTop: 0,
		marginBottom: 16,
	},

	"& p:only-child": {
		marginTop: 0,
		marginBottom: 0,
	},

	"& ul, & ol": {
		display: "flex",
		flexDirection: "column",
		gap: 8,
		marginBottom: 16,
	},

	"& li > ul, & li > ol": {
		marginTop: 16,
	},

	"& li > p": {
		marginBottom: 0,
	},

	"& .prismjs": {
		background: theme.palette.background.paper,
		borderRadius: 8,
		padding: "16px 24px",
		overflowX: "auto",

		"& code": {
			color: theme.palette.text.secondary,
		},

		"& .key, & .property, & .inserted, .keyword": {
			color: colors.teal[300],
		},

		"& .deleted": {
			color: theme.palette.error.light,
		},
	},
});
