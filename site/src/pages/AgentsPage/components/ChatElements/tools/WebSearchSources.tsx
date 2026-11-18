import { cn } from "cn";
import { ExternalLinkIcon, GlobeIcon } from "lucide-react";
import { useRef } from "react";
import { NewTabLabel } from "#/components/Link/Link";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import type { SourceLink } from "../../ChatConversation/types";

type WebSearchSourcesProps = {
	sources: SourceLink[];
};

/** Cited pages, with non-web URLs kept as readable text instead of links. */
const WebSearchSources: React.FC<WebSearchSourcesProps> = ({ sources }) => {
	const firstLinkRef = useRef<HTMLAnchorElement>(null);
	// Deduplicate sources by URL, keeping the first occurrence.
	const unique = (() => {
		const seen = new Set<string>();
		return sources.filter((s) => {
			if (!s.url || seen.has(s.url)) {
				return false;
			}
			seen.add(s.url);
			return true;
		});
	})();

	if (unique.length === 0) {
		return null;
	}

	const firstWebSource = unique.find((source) => getSourceDisplay(source).href);

	return (
		<Popover>
			<PopoverTrigger
				aria-label={`Sources (${unique.length.toLocaleString("en-US")})`}
				className="group/sources inline-flex h-6 items-center gap-1.5 rounded-full bg-surface-secondary pl-0.5 pr-2 text-xs text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
			>
				<span aria-hidden className="inline-flex items-center">
					{unique.slice(0, 3).map((source) => {
						const { href, hostname } = getSourceDisplay(source);
						return (
							<span
								key={source.url}
								className="relative inline-flex size-5 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-secondary ring-1 ring-surface-secondary transition not-first:-ml-1.5 group-hover/sources:bg-surface-tertiary group-hover/sources:ring-surface-tertiary"
							>
								<GlobeIcon className="size-3" />
								{href && hostname && (
									<img
										src={`https://www.google.com/s2/favicons?domain=${encodeURIComponent(hostname)}&sz=16`}
										alt=""
										width={16}
										height={16}
										className="absolute size-4"
										onError={(event) => {
											event.currentTarget.style.display = "none";
										}}
									/>
								)}
							</span>
						);
					})}
					{unique.length > 3 && (
						<span className="pl-1 text-2xs">
							+{(unique.length - 3).toLocaleString("en-US")}
						</span>
					)}
				</span>
				Sources
			</PopoverTrigger>
			<PopoverContent
				aria-label="Sources"
				side="top"
				align="start"
				className="w-80 max-w-(--radix-popper-available-width) p-2"
				onOpenAutoFocus={(event) => {
					if (firstLinkRef.current) {
						event.preventDefault();
						firstLinkRef.current.focus();
					}
				}}
			>
				<ul className="flex flex-col gap-2">
					{unique.map((source) => (
						<li key={source.url}>
							<SourceListItem
								source={source}
								linkRef={source === firstWebSource ? firstLinkRef : undefined}
							/>
						</li>
					))}
				</ul>
			</PopoverContent>
		</Popover>
	);
};

/**
 * Derives a source's label: the title, or the hostname plus path
 * when there is no title, so pages from one site stay distinguishable.
 * href is set only for http and https URLs, because provider URLs with
 * other schemes must not become links.
 */
const getSourceDisplay = (
	source: SourceLink,
): { label: string; href: string | undefined; hostname: string } => {
	let url: URL | undefined;
	try {
		url = new URL(source.url);
	} catch {
		url = undefined;
	}
	const isWebUrl = url?.protocol === "http:" || url?.protocol === "https:";
	const hostname = url?.hostname ?? "";
	const path = url && url.pathname !== "/" ? url.pathname : "";
	return {
		label: source.title || (hostname ? `${hostname}${path}` : source.url),
		href: isWebUrl ? source.url : undefined,
		hostname,
	};
};

/**
 * A source URL pill with a favicon from Google's S2 service and a
 * truncated label from getSourceDisplay. URLs without an http or https
 * scheme render as text instead of a link.
 */
export const SourcePill: React.FC<{ source: SourceLink }> = ({ source }) => {
	const { label, href, hostname } = getSourceDisplay(source);
	const className = cn(
		"inline-flex items-center gap-1.5 rounded-full",
		"border bg-surface-secondary",
		"px-2.5 py-1 text-xs leading-none text-content-secondary",
		"max-w-[200px]",
	);

	if (!href) {
		return (
			<span title={source.url} className={className}>
				<span className="truncate">{label}</span>
			</span>
		);
	}

	return (
		<a
			href={href}
			target="_blank"
			rel="noopener noreferrer"
			title={source.title || source.url}
			className={cn(
				className,
				"no-underline transition-colors",
				"hover:bg-surface-tertiary hover:text-content-primary",
				"hover:border-border-secondary",
			)}
		>
			{hostname && (
				<img
					src={`https://www.google.com/s2/favicons?domain=${encodeURIComponent(hostname)}&sz=16`}
					alt=""
					width={14}
					height={14}
					className="shrink-0 rounded-sm"
					// Hide the broken-image icon if the favicon fails to load.
					onError={(e) => {
						e.currentTarget.style.display = "none";
					}}
				/>
			)}
			<span className="truncate">{label}</span>
			<ExternalLinkIcon aria-hidden className="size-3 shrink-0" />
			<NewTabLabel />
		</a>
	);
};

export default WebSearchSources;

type SourceListItemProps = {
	source: SourceLink;
	linkRef: React.Ref<HTMLAnchorElement> | undefined;
};

const SourceListItem: React.FC<SourceListItemProps> = ({ source, linkRef }) => {
	const { label, href, hostname } = getSourceDisplay(source);
	const Component = href ? "a" : "span";
	return (
		<Component
			ref={href ? linkRef : undefined}
			href={href}
			target={href ? "_blank" : undefined}
			rel={href ? "noopener noreferrer" : undefined}
			className={cn(
				"flex items-start gap-2 rounded-md p-2 text-content-primary no-underline",
				href &&
					"hover:bg-surface-tertiary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
			)}
		>
			<GlobeIcon
				aria-hidden
				className="mt-0.5 size-4 shrink-0 text-content-secondary"
			/>
			<span className="flex min-w-0 flex-1 flex-col gap-1">
				<span className="wrap-anywhere text-sm">{label}</span>
				<span className="wrap-anywhere text-xs text-content-secondary">
					{href ? hostname : source.url}
				</span>
			</span>
			{href && (
				<>
					<ExternalLinkIcon
						aria-hidden
						className="mt-0.5 size-3 shrink-0 text-content-secondary"
					/>
					<NewTabLabel />
				</>
			)}
		</Component>
	);
};
