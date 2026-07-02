import { ExternalLinkIcon, GlobeIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "#/utils/cn";
import { ToolCall } from "./ToolCall";

interface WebSearchSourcesProps {
	sources: Array<{ url: string; title: string }>;
}

/** Collapsible web-search result pills, styled as a ToolCall row. */
const WebSearchSources: FC<WebSearchSourcesProps> = ({ sources }) => {
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

	const detail = unique.length === 1 ? "1 result" : `${unique.length} results`;

	return (
		<ToolCall.Root status="completed" hasContent={unique.length > 0}>
			<ToolCall.HeaderButton>
				<ToolCall.LeadingIcon>
					<GlobeIcon className="size-4 shrink-0 stroke-[1.5] text-current" />
				</ToolCall.LeadingIcon>
				<ToolCall.Label>
					Searched <span className="text-content-secondary/60">{detail}</span>
				</ToolCall.Label>
				<ToolCall.Chevron />
			</ToolCall.HeaderButton>
			<ToolCall.Content>
				<div className="mt-1.5 flex flex-wrap items-center gap-1.5">
					{unique.map((source) => (
						<SourcePill key={source.url} source={source} />
					))}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
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
				"hover:border-border-secondary",
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
			<ExternalLinkIcon className="size-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
		</a>
	);
};

export default WebSearchSources;
