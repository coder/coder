import Link from "@mui/material/Link";
import isEqual from "lodash/isEqual";
import { type FC, memo } from "react";
import ReactMarkdown, { type Options } from "react-markdown";

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
 * Supports a strict subset of Markdown that behaves well as inline/confined
 * content. Separated from the full Markdown component so that importing it
 * does not pull in the heavy PrismJS syntax-highlighting bundle.
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
						className="rounded-sm bg-border px-1 py-px text-[14px] text-content-primary"
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

export const MemoizedInlineMarkdown = memo(InlineMarkdown, isEqual);
