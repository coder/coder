import defaultMdxComponents from "fumadocs-ui/mdx";
import { Accordion, Accordions } from "fumadocs-ui/components/accordion";
import { Tab, Tabs } from "fumadocs-ui/components/tabs";
import { OSTab } from "./os-tab";
import type { MDXComponents } from "mdx/types";
import type { ImgHTMLAttributes, ReactNode } from "react";

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
function LocalImg({ src, alt, ...props }: ImgHTMLAttributes<HTMLImageElement>) {
	const url = typeof src === "string" ? src : undefined;
	return <img {...props} src={url} alt={alt ?? ""} loading="lazy" />;
}

export function getMDXComponents(components?: MDXComponents) {
	return {
		...defaultMdxComponents,
		...stubComponents,
		Accordions,
		Accordion,
		Tabs,
		Tab,
		OSTab,
		img: LocalImg,
		...components,
	} satisfies MDXComponents;
}

export const useMDXComponents = getMDXComponents;

declare global {
	type MDXProvidedComponents = ReturnType<typeof getMDXComponents>;
}
