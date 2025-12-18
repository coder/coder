import type React from "react";
import { useEffect, useState } from "react";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import {
	darcula,
	dracula,
} from "react-syntax-highlighter/dist/cjs/styles/prism";

export const Codeblock = ({
	children,
	...props
}: React.ComponentProps<typeof SyntaxHighlighter>) => {
	const [isDark, setIsDark] = useState(() => {
		if (typeof document === "undefined") {
			return true;
		}
		return document.documentElement.classList.contains("dark");
	});

	useEffect(() => {
		if (typeof document === "undefined") {
			return;
		}
		const root = document.documentElement;
		const update = () => setIsDark(root.classList.contains("dark"));
		update();

		const mo = new MutationObserver(update);
		mo.observe(root, { attributes: true, attributeFilter: ["class"] });
		return () => {
			mo.disconnect();
		};
	}, []);

	return (
		<SyntaxHighlighter
			language="json"
			style={isDark ? dracula : darcula}
			customStyle={{
				padding: 0,
				border: "none",
				background: "transparent",
			}}
			{...props}
		>
			{children}
		</SyntaxHighlighter>
	);
};
