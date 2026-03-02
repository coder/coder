import { useTheme } from "@emotion/react";
import {
	File as FileViewer,
	type SupportedLanguages,
} from "@pierre/diffs/react";
import type { ComponentPropsWithRef, ReactNode } from "react";
import { useMemo } from "react";
import { type Components, Streamdown } from "streamdown";
import { cn } from "utils/cn";

interface ResponseProps extends Omit<ComponentPropsWithRef<"div">, "children"> {
	children: string;
}

const fileViewerCSS =
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; }";

const fileViewerTheme = {
	light: "github-light",
	dark: "github-dark-high-contrast",
} as const;

type HastNode = {
	type?: string;
	value?: string;
	children?: HastNode[];
	tagName?: string;
	properties?: {
		className?: string[] | string;
	};
};

type MarkdownComponentProps = {
	href?: string;
	children?: ReactNode;
	node?: HastNode;
};

type FileViewerThemeType = "light" | "dark";

/**
 * Recursively extracts text from a HAST node tree. This is plain
 * data (not React elements), so it's reliable to traverse.
 */
const getHastText = (node: HastNode | null | undefined): string => {
	if (!node) {
		return "";
	}
	if (node.type === "text") return node.value ?? "";
	if (node.children) return node.children.map(getHastText).join("");
	return "";
};

const getClassNames = (className: string[] | string | undefined): string[] => {
	if (typeof className === "string") {
		return className.split(/\s+/).filter(Boolean);
	}
	if (!Array.isArray(className)) {
		return [];
	}
	return className.filter(
		(classToken): classToken is string => typeof classToken === "string",
	);
};

const createComponents = (
	fileViewerThemeType: FileViewerThemeType,
	viewerTheme: (typeof fileViewerTheme)[FileViewerThemeType],
): Components => {
	return {
		a: ({ href, children }: MarkdownComponentProps) => (
			<a
				href={href}
				target="_blank"
				rel="noopener noreferrer"
				className="text-content-link no-underline hover:underline hover:decoration-content-link"
			>
				{children}
			</a>
		),
		// Inline code only — fenced blocks are handled by the pre override.
		code: ({ children }: MarkdownComponentProps) => (
			<code className="rounded bg-surface-quaternary/25 px-1 py-0.5 font-mono text-content-primary">
				{children}
			</code>
		),
		// Fenced code blocks: extract language and content from the HAST
		// node directly (plain data), then render with FileViewer.
		pre: ({ node }: MarkdownComponentProps) => {
			const codeChild = node?.children?.[0];
			if (codeChild?.tagName === "code") {
				const classes = getClassNames(codeChild.properties?.className);
				const langClass = classes.find((c: string) =>
					c.startsWith("language-"),
				);
				const lang = langClass ? langClass.replace("language-", "") : "text";
				const content = getHastText(codeChild).trimEnd();
				if (content) {
					return (
						<div className="my-4 overflow-hidden rounded-xl border border-solid border-border-default text-2xs">
							<FileViewer
								file={{
									name: `block.${lang}`,
									lang: lang as SupportedLanguages,
									contents: content,
									cacheKey: content,
								}}
								options={{
									overflow: "scroll",
									themeType: fileViewerThemeType,
									disableFileHeader: true,
									disableLineNumbers: true,
									theme: viewerTheme,
									unsafeCSS: fileViewerCSS,
								}}
							/>
						</div>
					);
				}
			}
			return <pre>{node?.children?.map?.(() => null)}</pre>;
		},
	};
};

export const Response = ({
	className,
	children,
	ref,
	...props
}: ResponseProps) => {
	const theme = useTheme();
	const fileViewerThemeType: FileViewerThemeType =
		theme.palette.mode === "dark" ? "dark" : "light";
	const viewerTheme = fileViewerTheme[fileViewerThemeType];
	const components = useMemo(
		() => createComponents(fileViewerThemeType, viewerTheme),
		[fileViewerThemeType, viewerTheme],
	);

	return (
		<div
			ref={ref}
			className={cn(
				"text-[13px] leading-relaxed text-content-primary",
				className,
			)}
			{...props}
		>
			<Streamdown controls={false} components={components}>
				{children}
			</Streamdown>
		</div>
	);
};
