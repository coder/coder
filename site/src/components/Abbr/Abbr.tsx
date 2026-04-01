import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";

type Pronunciation = "shorthand" | "acronym" | "initialism";

type AbbrProps = HTMLAttributes<HTMLElement> & {
	children: string;
	title: string;
	pronunciation?: Pronunciation;
	className?: string;
};

/**
 * A more sophisticated version of the native <abbr> element.
 *
 * Features:
 * - Better type-safety (requiring you to include certain properties)
 * - All built-in HTML styling is stripped away by default
 * - Better integration with screen readers (like exposing the title prop to
 *   them), with more options for influencing how they pronounce text
 */
export const Abbr: FC<AbbrProps> = ({
	children,
	title,
	pronunciation = "shorthand",
	className,
	...delegatedProps
}) => {
	return (
		<abbr
			// Adding title to make things a little easier for sighted users,
			// but titles aren't always exposed via screen readers. Still have
			// to inject the actual text content inside the abbr itself
			title={title}
			className={cn(
				"no-underline tracking-normal",
				children === children.toUpperCase() && "tracking-wide",
				className,
			)}
			{...delegatedProps}
		>
			<span aria-hidden>{children}</span>
			<span className="sr-only">
				{getAccessibleLabel(children, title, pronunciation)}
			</span>
		</abbr>
	);
};

function getAccessibleLabel(
	abbreviation: string,
	title: string,
	pronunciation: Pronunciation,
): string {
	if (pronunciation === "initialism") {
		return `${initializeText(abbreviation)} (${title})`;
	}

	if (pronunciation === "acronym") {
		return `${flattenPronunciation(abbreviation)} (${title})`;
	}

	return title;
}

function initializeText(text: string): string {
	return `${text.trim().toUpperCase().replaceAll(/\B/g, ".")}.`;
}

function flattenPronunciation(text: string): string {
	const trimmed = text.trim();
	return (trimmed[0] ?? "").toUpperCase() + trimmed.slice(1).toLowerCase();
}
