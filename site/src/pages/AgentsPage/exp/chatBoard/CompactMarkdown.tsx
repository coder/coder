import { cn } from "cn";
import type { FC } from "react";
import { Markdown } from "#/components/Markdown/Markdown";

type CompactMarkdownProps = {
	readonly className?: string;
	readonly children: string;
};

/** Markdown with block spacing tightened for small text inside a card or popover. */
export const CompactMarkdown: FC<CompactMarkdownProps> = ({
	className,
	children,
}) => (
	<Markdown
		className={cn(
			"wrap-anywhere [&_p]:mt-0 [&_p]:mb-0 [&_p+p]:mt-1 [&_ul]:my-1 [&_ol]:my-1 [&_ul]:gap-0.5 [&_ol]:gap-0.5 [&_ul]:list-disc [&_ol]:list-decimal [&_ul]:pl-4 [&_ol]:pl-4 [&_li>ul]:mt-0.5 [&_li>ol]:mt-0.5 [&_code]:text-[length:inherit] [&_pre]:my-1 [&_pre]:overflow-x-auto [&_pre]:text-[11px]",
			className,
		)}
	>
		{children}
	</Markdown>
);
