import frontMatter from "front-matter";
import { LinkIcon, ListIcon } from "lucide-react";
import { createElement, useEffect, useRef, useState } from "react";
import type { Components } from "react-markdown";
import { Link, useLocation, useSearchParams } from "react-router";
import rehypeSlug from "rehype-slug";
import type { TemplateVersionDocumentation } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Markdown } from "#/components/Markdown/Markdown";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";

const MarkdownHeading: Components["h1"] = ({ node, children, id }) => {
	const { search } = useLocation();
	return createElement(
		node?.tagName ?? "h1",
		{ id, tabIndex: -1, className: "group relative scroll-mt-24" },
		<Link
			to={{ search, hash: `#${id}` }}
			aria-label={`Link to ${id}`}
			className="absolute -left-6 top-1/2 -translate-y-1/2 p-1 text-content-secondary hover:text-content-primary focus-visible:outline focus-visible:outline-2"
		>
			<LinkIcon
				aria-hidden="true"
				className="size-4 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
			/>
		</Link>,
		children,
	);
};

interface Heading {
	id: string;
	text: string;
	level: number;
}

export const TemplateDocsPageView = ({
	files,
}: TemplateVersionDocumentation) => {
	const [params, setParams] = useSearchParams();
	const location = useLocation();
	const names = Object.keys(files).sort((a, b) => {
		return (
			Number(b.toLowerCase() === "readme.md") -
				Number(a.toLowerCase() === "readme.md") || a.localeCompare(b)
		);
	});
	const selected = params.get("tab") ?? names[0];
	const body =
		selected && Object.hasOwn(files, selected)
			? frontMatter(files[selected]).body
			: undefined;
	const contentRef = useRef<HTMLDivElement>(null);
	const [tocOpen, setTocOpen] = useState(false);
	const navigatingFromToc = useRef(false);
	const [headings, setHeadings] = useState<Heading[]>([]);

	useEffect(() => {
		if (!selected || !body || !location.hash) return;
		let id: string;
		try {
			id = decodeURIComponent(location.hash.slice(1));
		} catch {
			return;
		}
		const target = document.getElementById(id);
		if (target && contentRef.current?.contains(target)) {
			target.scrollIntoView();
			target.focus({ preventScroll: true });
		}
	}, [selected, body, location]);

	if (names.length === 0)
		return <p>This template has no Markdown documentation.</p>;

	return (
		<Tabs
			value={selected}
			className="rounded-lg border border-solid border-border bg-surface-primary"
			onValueChange={(name) => {
				const search = new URLSearchParams(params);
				search.set("tab", name);
				setParams(search);
				setTocOpen(false);
			}}
		>
			<div className="flex items-center gap-4 border-b border-border px-6">
				<TabsList aria-label="Template documents" className="flex-1 border-0">
					{names.map((name) => (
						<TabsTrigger key={name} value={name}>
							{name}
						</TabsTrigger>
					))}
				</TabsList>
				<Popover
					open={tocOpen}
					onOpenChange={(open) => {
						if (open) {
							setHeadings(
								Array.from(
									contentRef.current?.querySelectorAll(
										"h1, h2, h3, h4, h5, h6",
									) ?? [],
								).map((el) => ({
									id: el.id,
									text: el.textContent ?? "",
									level: Number(el.tagName.slice(1)),
								})),
							);
						}
						setTocOpen(open);
					}}
				>
					<PopoverTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							aria-label="Table of contents"
							disabled={!body}
						>
							<ListIcon />
						</Button>
					</PopoverTrigger>
					<PopoverContent
						align="end"
						className="p-3"
						onCloseAutoFocus={(event) => {
							if (navigatingFromToc.current) {
								event.preventDefault();
								navigatingFromToc.current = false;
							}
						}}
					>
						<nav aria-label="Table of contents">
							{headings.length === 0 ? (
								<p>No headings in this document.</p>
							) : (
								<ul className="m-0 list-none p-0">
									{headings.map((item) => (
										<li
											key={item.id}
											style={{ paddingLeft: (item.level - 1) * 12 }}
										>
											<Link
												className="block py-1 text-sm text-content-primary"
												to={{ search: location.search, hash: `#${item.id}` }}
												onClick={() => {
													navigatingFromToc.current = true;
													setTocOpen(false);
												}}
											>
												{item.text}
											</Link>
										</li>
									))}
								</ul>
							)}
						</nav>
					</PopoverContent>
				</Popover>
			</div>
			{body === undefined ? (
				<p className="p-6">Document not found. Select a document above.</p>
			) : (
				<TabsContent value={selected ?? ""} className="px-8 pt-2 pb-12">
					<div ref={contentRef} className="max-w-[860px]">
						<Markdown
							rehypePlugins={[rehypeSlug]}
							components={{
								h1: MarkdownHeading,
								h2: MarkdownHeading,
								h3: MarkdownHeading,
								h4: MarkdownHeading,
								h5: MarkdownHeading,
								h6: MarkdownHeading,
							}}
						>
							{body}
						</Markdown>
					</div>
				</TabsContent>
			)}
		</Tabs>
	);
};
