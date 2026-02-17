import {
	File as FileViewer,
	type SupportedLanguages,
} from "@pierre/diffs/react";
import { forwardRef } from "react";
import { Streamdown } from "streamdown";
import { cn } from "utils/cn";

interface ResponseProps
	extends Omit<React.HTMLAttributes<HTMLDivElement>, "children"> {
	children: string;
}

const fileViewerCSS =
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; }";

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
	children?: React.ReactNode;
	node?: HastNode;
};

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

const components: Record<
	string,
	React.ComponentType<MarkdownComponentProps>
> = {
	a: ({ href, children }: MarkdownComponentProps) => (
		<a
			href={href}
			target="_blank"
			rel="noopener noreferrer"
			className="text-content-link hover:underline hover:decoration-content-link"
			style={{ textDecoration: "none" }}
		>
			{children}
		</a>
	),
	// Inline code only — fenced blocks are handled by the pre override.
	code: ({ children }: MarkdownComponentProps) => (
		<code className="rounded bg-surface-quaternary/25 px-1 py-0.5 font-mono text-[#FFB757]">
			{children}
		</code>
	),
	// Fenced code blocks: extract language and content from the HAST
	// node directly (plain data), then render with FileViewer.
	pre: ({ node }: MarkdownComponentProps) => {
		const codeChild = node?.children?.[0];
		if (codeChild?.tagName === "code") {
			const className = codeChild.properties?.className;
			const classes =
				typeof className === "string"
					? className.split(/\s+/).filter(Boolean)
					: Array.isArray(className)
						? className.filter(
								(classToken): classToken is string =>
									typeof classToken === "string",
							)
						: [];
			const langClass = classes.find((c: string) => c.startsWith("language-"));
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
							}}
							options={{
								overflow: "scroll",
								themeType: "dark",
								disableFileHeader: true,
								theme: "github-dark-high-contrast",
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

export const Response = forwardRef<HTMLDivElement, ResponseProps>(
	({ className, children, ...props }, ref) => {
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
	},
);

Response.displayName = "Response";
