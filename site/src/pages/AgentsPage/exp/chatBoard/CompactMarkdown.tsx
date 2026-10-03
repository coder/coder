import { cn } from "cn";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import { Markdown } from "#/components/Markdown/Markdown";

type CompactMarkdownProps = {
	readonly className?: string;
	readonly children: string;
};

/** Markdown with block spacing tightened for small text inside a card or popover. */
export const CompactMarkdown: React.FC<CompactMarkdownProps> = ({
	className,
	children,
}) => (
	<Markdown
		className={cn(
			"wrap-anywhere [&_p]:mt-0 [&_p]:mb-0 [&_p+p]:mt-1 [&_ul]:my-1 [&_ol]:my-1 [&_ul]:gap-0.5 [&_ol]:gap-0.5 [&_ul]:list-disc [&_ol]:list-decimal [&_ul]:pl-4 [&_ol]:pl-4 [&_li>ul]:mt-0.5 [&_li>ol]:mt-0.5 [&_code]:text-[length:inherit] [&_pre]:my-1 [&_pre]:overflow-x-auto [&_pre]:text-[11px]",
			className,
		)}
		components={{ a: InlineLink }}
	>
		{children}
	</Markdown>
);

// The shared Link is an inline-flex box, so a link that wraps in a narrow
// card turns into a block with its icon floating at the side and its hover
// underline under the whole box. An inline anchor wraps like the text around
// it and underlines each line.
const InlineLink: React.FC<React.ComponentProps<"a">> = ({
	href,
	children,
}) => {
	const external = href?.startsWith("http");
	return (
		<a
			href={href}
			target={external ? "_blank" : undefined}
			rel={external ? "noreferrer" : undefined}
			className="font-medium text-content-link no-underline hover:underline focus-visible:rounded-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
		>
			{external ? <WithTrailingIcon>{children}</WithTrailingIcon> : children}
		</a>
	);
};

// Browsers may wrap before an inline icon, leaving it alone on a new line.
// Binding it to the last character keeps it after the text; one character
// rather than the last word, so a long URL can still wrap.
const WithTrailingIcon: React.FC<{ readonly children: React.ReactNode }> = ({
	children,
}) => {
	const icon = (
		<SquareArrowOutUpRightIcon
			aria-hidden="true"
			className="ml-0.5 inline size-3 align-[-1px]"
		/>
	);
	if (typeof children !== "string" || children.length === 0) {
		return (
			<>
				{children}
				{icon}
			</>
		);
	}
	return (
		<>
			{children.slice(0, -1)}
			<span className="whitespace-nowrap">
				{children.slice(-1)}
				{icon}
			</span>
		</>
	);
};
