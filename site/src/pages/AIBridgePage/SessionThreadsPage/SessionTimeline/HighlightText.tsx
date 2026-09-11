import type { FC } from "react";
import { splitMatchSegments } from "./sessionSearch";

type HighlightTextProps = {
	text: string;
	highlight: string;
};

// Renders text with query matches bolded in the primary color.
export const HighlightText: FC<HighlightTextProps> = ({ text, highlight }) => {
	const segments = splitMatchSegments(text, highlight);
	if (segments.length === 1 && !segments[0].match) {
		return text;
	}
	return (
		<>
			{segments.map((segment, i) =>
				segment.match ? (
					<strong key={i} className="text-content-primary font-semibold">
						{segment.text}
					</strong>
				) : (
					<span key={i}>{segment.text}</span>
				),
			)}
		</>
	);
};
