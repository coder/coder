import { type FC, memo } from "react";
import ReactMarkdown from "react-markdown";
import gfm from "remark-gfm";
import { cn } from "utils/cn";

interface ChatMarkdownProps {
	/** The raw Markdown string to render. */
	children: string;
	className?: string;
}

/**
 * Compact Markdown renderer sized for the AI chat sidebar.
 *
 * Uses @tailwindcss/typography `prose-sm` with tightened spacing
 * overrides so headings, lists, and code blocks feel at home in a
 * narrow, dense chat context rather than a full-page article.
 */
const ChatMarkdown: FC<ChatMarkdownProps> = ({ children, className }) => {
	return (
		<ReactMarkdown
			remarkPlugins={[gfm]}
			className={cn(
				// Base prose — small variant, inherits panel text color.
				"prose prose-sm max-w-none break-words",
				// Tighten vertical rhythm for chat context.
				"prose-p:my-1 prose-p:leading-relaxed",
				"prose-headings:mt-3 prose-headings:mb-1",
				"prose-ul:my-1 prose-ol:my-1 prose-li:my-0",
				"prose-pre:my-1.5 prose-pre:rounded-md prose-pre:bg-surface-secondary",
				// Inline code.
				"prose-code:rounded prose-code:bg-surface-secondary prose-code:px-1 prose-code:py-0.5",
				"prose-code:before:content-none prose-code:after:content-none",
				// Links.
				"prose-a:text-content-link prose-a:no-underline hover:prose-a:underline",
				className,
			)}
		>
			{children}
		</ReactMarkdown>
	);
};

export const MemoizedChatMarkdown = memo(ChatMarkdown);
