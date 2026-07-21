import defaultMdxComponents from "fumadocs-ui/mdx";
import { Accordion, Accordions } from "fumadocs-ui/components/accordion";
import { ArrowSquareOut } from "@phosphor-icons/react/ssr";
import { ImageZoom } from "fumadocs-ui/components/image-zoom";
import { Tab, Tabs } from "fumadocs-ui/components/tabs";
import { OSTab } from "./os-tab";
import type { MDXComponents } from "mdx/types";
import type {
	AnchorHTMLAttributes,
	ElementType,
	ImgHTMLAttributes,
	ReactNode,
} from "react";

/**
 * Render children inline and ignore unknown attributes. Used as a stub for
 * Coder-specific tags that have no Fumadocs equivalent.
 */
function Passthrough({ children }: { children?: ReactNode }) {
	return <>{children}</>;
}

/**
 * MDX safety, defense-in-depth.
 *
 * The PRIMARY guard against build breakage is the content pipeline: the sync
 * step emits `.md` (not `.mdx`), and Fumadocs MDX compiles `.md` as plain
 * Markdown with NO JSX/expression parsing. So the corpus's raw custom tags
 * (`<children>`, `<workspace>`, ...) and literal `{...}` never reach the JSX
 * parser and cannot fail the build; in `.md` mode they resolve to inert raw
 * HTML elements.
 *
 * These stub components are a SECONDARY guard: if any document is ever treated
 * as MDX (renamed to `.mdx`, or a future config change), the unknown tags
 * resolve to an inert passthrough instead of throwing
 * "Expected component `X` to be defined".
 */
const stubComponents: MDXComponents = {
	children: Passthrough,
	Children: Passthrough,
};

// Doc images are copied into the bundle by the sync step and referenced by
// root-absolute paths (/images/...). Render a plain <img>: next/image needs a
// running server to optimize, which a static export does not have, and the
// bundle is meant to work fully offline.
//
// ImageZoom makes every image click-to-zoom. Passing the plain <img> as its
// children renders that <img> directly and skips ImageZoom's internal
// next/image code path, so the static, unoptimized image keeps working while
// the zoomed-in overlay still loads the same local source.
function LocalImg({ src, alt, ...props }: ImgHTMLAttributes<HTMLImageElement>) {
	const url = typeof src === "string" ? src : undefined;
	return (
		<ImageZoom src={url}>
			<img {...props} src={url} alt={alt ?? ""} loading="lazy" />
		</ImageZoom>
	);
}

// External links open in a new tab, flagged with an icon so it is clear the
// link leaves the docs. Internal links delegate to `InternalLink` (the page
// passes Fumadocs' relative-link resolver as `a`), preserving client-side
// navigation and relative-path resolution. Fumadocs already opens external
// links in a new tab; this adds the trailing icon affordance.
function createDocsLink(InternalLink: ElementType) {
	return function DocsLink({
		href,
		children,
		...props
	}: AnchorHTMLAttributes<HTMLAnchorElement>) {
		const isExternal = typeof href === "string" && /^https?:\/\//i.test(href);
		if (!isExternal) {
			return (
				<InternalLink href={href} {...props}>
					{children}
				</InternalLink>
			);
		}
		return (
			<a href={href} target="_blank" rel="noopener noreferrer" {...props}>
				{children}
				<ArrowSquareOut
					aria-hidden
					className="ml-0.5 inline size-3.5 align-text-top"
				/>
			</a>
		);
	};
}

export function getMDXComponents(components?: MDXComponents) {
	// The docs page passes `a` as Fumadocs' relative-link resolver. Pull it out
	// and wrap it so external links get the new-tab + icon treatment while
	// internal links keep that resolver; set `a` last so the wrapper wins.
	const { a: providedLink, ...rest } = components ?? {};
	const InternalLink = (providedLink ??
		defaultMdxComponents.a ??
		"a") as ElementType;
	return {
		...defaultMdxComponents,
		...stubComponents,
		Accordions,
		Accordion,
		Tabs,
		Tab,
		OSTab,
		img: LocalImg,
		...rest,
		a: createDocsLink(InternalLink),
	} satisfies MDXComponents;
}

export const useMDXComponents = getMDXComponents;

declare global {
	type MDXProvidedComponents = ReturnType<typeof getMDXComponents>;
}
