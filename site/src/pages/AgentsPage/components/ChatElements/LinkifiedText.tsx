import type React from "react";
import { Fragment } from "react";
import { useChatUrlTransform } from "../../context/ChatUrlTransformContext";
import { splitTextForLinks } from "./linkify";

export const LinkifiedText: React.FC<{ text: string }> = ({ text }) => {
	const transform = useChatUrlTransform();
	const segments = splitTextForLinks(text);
	if (!segments.some((segment) => segment.kind === "url")) {
		return text;
	}
	return segments.map((segment, index) => {
		if (segment.kind === "text") {
			return <Fragment key={index}>{segment.value}</Fragment>;
		}
		return (
			<a
				key={index}
				href={transform ? transform(segment.value) : segment.value}
				target="_blank"
				rel="noopener noreferrer"
				className="font-[inherit] text-content-link underline underline-offset-2 hover:no-underline"
			>
				{segment.value}
			</a>
		);
	});
};
