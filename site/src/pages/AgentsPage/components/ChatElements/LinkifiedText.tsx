import type React from "react";
import { Fragment } from "react";
import type { UrlTransform } from "streamdown";
import { splitTextForLinks } from "./linkify";

export const LinkifiedText: React.FC<{
	text: string;
	transform?: UrlTransform;
}> = ({ text, transform }) => {
	const segments = splitTextForLinks(text);
	if (!segments.some((segment) => segment.kind === "url")) {
		return text;
	}
	return segments.map((segment, index) => {
		if (segment.kind === "text") {
			return <Fragment key={index}>{segment.value}</Fragment>;
		}
		const href =
			transform?.(segment.value, "href", {
				type: "element",
				tagName: "a",
				properties: { href: segment.value },
				children: [{ type: "text", value: segment.value }],
			}) ?? segment.value;
		return (
			<a
				key={index}
				href={href}
				target="_blank"
				rel="noopener noreferrer"
				className="font-[inherit] text-content-link underline underline-offset-2 hover:no-underline"
			>
				{segment.value}
			</a>
		);
	});
};
