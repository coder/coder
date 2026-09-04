import { cn } from "cn";
import type { FC } from "react";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";

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
				// biome-ignore lint: design-token: em-relative markdown heading spacing; needs design decision on tokenization
				"[&_h1]:mb-[0.75em] [&_h2]:mb-[0.75em] [&_h3]:mb-[0.75em]",
				// biome-ignore lint: design-token: em-relative markdown heading spacing; needs design decision on tokenization
				"[&_h4]:mb-[0.75em] [&_h5]:mb-[0.75em] [&_h6]:mb-[0.75em]",
				// biome-ignore lint: design-token: em-relative markdown heading sizes; needs design decision on tokenization
				"[&_h1]:text-[1.2em] [&_h2]:text-[1.15em] [&_h3]:text-[1.1em]",
				// biome-ignore lint: design-token: em-relative markdown heading sizes; needs design decision on tokenization
				"[&_h4]:text-[1.05em] [&_h5]:text-[1em] [&_h6]:text-[0.95em]",
			)}
		>
			{children}
		</MemoizedMarkdown>
	);
};
