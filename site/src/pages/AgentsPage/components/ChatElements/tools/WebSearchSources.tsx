import { cn } from "cn";
import { ExternalLinkIcon, GlobeIcon } from "lucide-react";
import type { FC } from "react";
import type { SourceLink } from "../../ChatConversation/types";
import { ToolCall } from "./ToolCall";

type WebSearchSourcesProps = {
	sources: SourceLink[];
};

/** Collapsible row of the answer's source pills, styled as a ToolCall row. */
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

	return (
		<ToolCall.Root status="completed" hasContent={unique.length > 0}>
			<ToolCall.HeaderButton>
				<ToolCall.LeadingIcon>
					<GlobeIcon className="size-4 shrink-0 stroke-[1.5] text-current" />
				</ToolCall.LeadingIcon>
				<ToolCall.Label>
					{unique.length === 1 ? "1 source" : `${unique.length} sources`}
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
 * Derives what a source pill shows: the title, or the hostname plus path
 * when there is no title, so pages from one site stay distinguishable.
 * href is set only for http and https URLs, because provider URLs with
 * other schemes must not become links.
 */
export const getSourcePillDisplay = (
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
 * truncated label from getSourcePillDisplay. URLs without an http or https
 * scheme render as text instead of a link.
 */
export const SourcePill: FC<{ source: SourceLink }> = ({ source }) => {
	const { label, href, hostname } = getSourcePillDisplay(source);
	const className = cn(
		"inline-flex items-center gap-1.5 rounded-full",
		"border border-solid border-border-default bg-surface-secondary",
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
				"group no-underline transition-colors",
				"hover:bg-surface-tertiary hover:text-content-primary",
				"hover:border-border-secondary",
			)}
		>
			{hostname && (
				<img
					src={`https://www.google.com/s2/favicons?domain=${hostname}&sz=16`}
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
			<ExternalLinkIcon className="size-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
		</a>
	);
};

export default WebSearchSources;
