import { ExternalLinkIcon } from "lucide-react";
import { type FC, useMemo } from "react";
import { cn } from "utils/cn";

interface WebSearchSourcesProps {
	sources: Array<{ url: string; title: string }>;
}

const WebSearchSources: FC<WebSearchSourcesProps> = ({ sources }) => {
	const unique = useMemo(() => {
		const seen = new Set<string>();
		return sources.filter((source) => {
			if (!source.url || seen.has(source.url)) {
				return false;
			}
			seen.add(source.url);
			return true;
		});
	}, [sources]);

	if (unique.length === 0) {
		return null;
	}

	return (
		<div className="flex flex-col gap-1.5">
			<span className="text-[11px] font-medium text-content-secondary">
				Sources
			</span>
			<div className="flex flex-wrap gap-1.5">
				{unique.map((source) => (
					<SourcePill key={source.url} source={source} />
				))}
			</div>
		</div>
	);
};

const SourcePill: FC<{ source: { url: string; title: string } }> = ({
	source,
}) => {
	let hostname = "";
	try {
		hostname = new URL(source.url).hostname.replace(/^www\./, "");
	} catch {
		hostname = "";
	}

	const label = source.title || hostname || source.url;

	return (
		<a
			href={source.url}
			target="_blank"
			rel="noopener noreferrer"
			title={source.title || source.url}
			className={cn(
				"inline-flex max-w-[220px] items-center gap-1 rounded-full",
				"bg-surface-secondary px-2.5 py-0.5 text-xs text-content-link",
				"no-underline transition-colors hover:bg-surface-tertiary",
				"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link/30",
			)}
		>
			<span className="truncate">{label}</span>
			<ExternalLinkIcon className="h-3 w-3 shrink-0" />
		</a>
	);
};

export default WebSearchSources;
