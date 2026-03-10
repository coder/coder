import { ChevronDownIcon, ExternalLinkIcon, GlobeIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { cn } from "utils/cn";

interface WebSearchSourcesProps {
	sources: Array<{ url: string; title: string }>;
}

/** Maximum number of source pills visible before collapsing. */
const VISIBLE_LIMIT = 4;

/**
 * Renders a compact row of citation pills for web search results.
 * Sources are deduplicated by URL and shown as small rounded chips
 * with favicons. When more than four sources are present, the rest
 * are hidden behind a "+N more" expander.
 */
const WebSearchSources: FC<WebSearchSourcesProps> = ({ sources }) => {
	const [expanded, setExpanded] = useState(false);

	// Deduplicate sources by URL, keeping the first occurrence.
	const unique = useMemo(() => {
		const seen = new Set<string>();
		return sources.filter((s) => {
			if (!s.url || seen.has(s.url)) {
				return false;
			}
			seen.add(s.url);
			return true;
		});
	}, [sources]);

	if (unique.length === 0) {
		return null;
	}

	const hasOverflow = unique.length > VISIBLE_LIMIT;
	const visible = expanded ? unique : unique.slice(0, VISIBLE_LIMIT);
	const hiddenCount = unique.length - VISIBLE_LIMIT;

	return (
		<div className="flex flex-col gap-1.5 py-1">
			{/* Label */}
			<span className="flex items-center gap-1 text-xs text-content-secondary">
				<GlobeIcon className="h-3 w-3 shrink-0" />
				Sources
			</span>

			{/* Pills */}
			<div className="flex flex-wrap items-center gap-1.5">
				{visible.map((source) => (
					<SourcePill key={source.url} source={source} />
				))}

				{hasOverflow && !expanded && (
					<button
						type="button"
						onClick={() => setExpanded(true)}
						className={cn(
							"inline-flex items-center gap-1 rounded-full",
							"border border-solid border-border-default bg-surface-secondary",
							"px-2.5 py-1 text-xs text-content-secondary",
							"cursor-pointer transition-colors",
							"hover:bg-surface-tertiary hover:text-content-primary",
							// Reset native button styles.
							"font-[inherit] m-0",
						)}
					>
						+{hiddenCount} more
					</button>
				)}

				{hasOverflow && expanded && (
					<button
						type="button"
						onClick={() => setExpanded(false)}
						className={cn(
							"inline-flex items-center gap-0.5 rounded-full",
							"border border-solid border-border-default bg-surface-secondary",
							"px-2.5 py-1 text-xs text-content-secondary",
							"cursor-pointer transition-colors",
							"hover:bg-surface-tertiary hover:text-content-primary",
							"font-[inherit] m-0",
						)}
					>
						Show less
						<ChevronDownIcon className="h-3 w-3 shrink-0 rotate-180" />
					</button>
				)}
			</div>
		</div>
	);
};

/**
 * A single source citation pill. Shows a favicon from Google's S2
 * service, a truncated title, and an external-link icon on hover.
 */
const SourcePill: FC<{ source: { url: string; title: string } }> = ({
	source,
}) => {
	let hostname: string;
	try {
		hostname = new URL(source.url).hostname;
	} catch {
		hostname = "";
	}

	const faviconUrl = hostname
		? `https://www.google.com/s2/favicons?domain=${hostname}&sz=16`
		: undefined;

	// Use the title if available, otherwise fall back to the hostname.
	const label = source.title || hostname || source.url;

	return (
		<a
			href={source.url}
			target="_blank"
			rel="noopener noreferrer"
			title={source.title || source.url}
			className={cn(
				"group inline-flex items-center gap-1.5 rounded-full",
				"border border-solid border-border-default bg-surface-secondary",
				"px-2.5 py-1 text-xs leading-none text-content-secondary",
				"no-underline transition-colors",
				"hover:bg-surface-tertiary hover:text-content-primary",
				"hover:border-border-hover",
				"max-w-[200px]",
			)}
		>
			{faviconUrl && (
				<img
					src={faviconUrl}
					alt=""
					width={14}
					height={14}
					className="shrink-0 rounded-sm"
					// Hide the broken-image icon if the favicon fails to load.
					onError={(e) => {
						(e.target as HTMLImageElement).style.display = "none";
					}}
				/>
			)}
			<span className="truncate">{label}</span>
			<ExternalLinkIcon className="h-3 w-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
		</a>
	);
};

export default WebSearchSources;
