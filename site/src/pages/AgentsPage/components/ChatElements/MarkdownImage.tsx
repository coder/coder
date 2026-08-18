import { ImageIcon } from "lucide-react";
import { useState } from "react";
import { cn } from "#/utils/cn";
import {
	externalImageHost,
	isExternalImageSource,
} from "#/utils/externalImageSources";

/**
 * Renders chat markdown images. External sources render a
 * click-to-load placeholder so viewing a chat never discloses the
 * viewer's IP to the image host (Cure53 CDM-02-006).
 */
export const MarkdownImage = ({ src, alt }: { src?: string; alt?: string }) => {
	const [consented, setConsented] = useState(false);

	if (!src) {
		return null;
	}

	if (consented || !isExternalImageSource(src)) {
		return (
			<img src={src} alt={alt ?? ""} loading="lazy" className="max-w-full" />
		);
	}

	const host = externalImageHost(src);
	// Sources without a resolvable host (for example javascript: or
	// otherwise malformed URLs) are never safe to load, so they get a
	// placeholder without a load affordance.
	if (!host) {
		return (
			<span className="inline-flex items-center gap-1.5 rounded-md border border-solid border-border-default bg-surface-secondary px-2 py-1 text-xs text-content-secondary">
				<ImageIcon aria-hidden className="size-3.5 shrink-0" />
				Blocked image{alt ? `: ${alt}` : ""}
			</span>
		);
	}

	return (
		<button
			type="button"
			onClick={() => setConsented(true)}
			aria-label={`Load external image from ${host}`}
			className={cn(
				"inline-flex max-w-full cursor-pointer items-center gap-1.5",
				"rounded-md border border-solid border-border-default bg-surface-secondary",
				"px-2 py-1 text-xs text-content-secondary",
				"hover:bg-surface-tertiary hover:text-content-primary",
			)}
		>
			<ImageIcon aria-hidden className="size-3.5 shrink-0" />
			<span className="truncate">
				{alt ? `${alt}: ` : ""}external image from {host}
			</span>
			<span className="shrink-0 font-medium text-content-link">Load</span>
		</button>
	);
};
