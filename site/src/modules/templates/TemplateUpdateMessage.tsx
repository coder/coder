import type { FC } from "react";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import { cn } from "#/utils/cn";

interface TemplateUpdateMessageProps {
	children: string;
}

export const TemplateUpdateMessage: FC<TemplateUpdateMessageProps> = ({
	children,
}) => {
	return (
		<MemoizedMarkdown
			className={cn(
				"text-sm leading-[1.2]",
				"[&_h1]:mb-[0.75em] [&_h2]:mb-[0.75em] [&_h3]:mb-[0.75em]",
				"[&_h4]:mb-[0.75em] [&_h5]:mb-[0.75em] [&_h6]:mb-[0.75em]",
				"[&_h1]:text-[1.2em] [&_h2]:text-[1.15em] [&_h3]:text-[1.1em]",
				"[&_h4]:text-[1.05em] [&_h5]:text-[1em] [&_h6]:text-[0.95em]",
			)}
		>
			{children}
		</MemoizedMarkdown>
	);
};
