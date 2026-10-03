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
//
// The external icon is a faded hint, smaller than the text: in notes nearly
// every link is external, so a full-strength icon would repeat on every line.
// It is drawn out of flow in the anchor's end padding. Inline padding only
// lands on the last line, so the icon cannot wrap onto a line of its own,
// whatever the label contains. It comes before the label in the DOM because
// Chrome treats an out-of-flow child after the text as a break point, which
// would let the padding wrap alone.
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
			className={cn(
				"font-medium text-content-link no-underline hover:underline focus-visible:rounded-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
				external && "relative pe-3.5",
			)}
		>
			{external && (
				<SquareArrowOutUpRightIcon
					aria-hidden="true"
					className="absolute end-0 bottom-[3px] size-2.5 opacity-55"
				/>
			)}
			{children}
		</a>
	);
};
