import { useLayoutEffect, useState } from "react";
import { parseURL } from "#/utils/uri";

/**
 * A frame that a Content-Security-Policy `frame-src` directive on the
 * dashboard page blocked.
 */
export type FrameBlock = {
	src: string;
	origin: string;
	/** True when only Coder's own policy blocked the frame. */
	byCoderPolicy: boolean;
};

/**
 * Returns the block when the page's CSP blocked a frame that loads `src`.
 * The page's CSP does not change until the page reloads, so a block stays
 * until `src` changes.
 */
export const useFrameBlock = (
	src: string | undefined,
): FrameBlock | undefined => {
	const [block, setBlock] = useState<FrameBlock>();

	// A layout effect subscribes in the commit that inserts the frame, before
	// the browser can report a violation for the frame's first navigation.
	useLayoutEffect(() => {
		if (src === undefined) {
			return;
		}
		const origin = parseURL(src, location.href)?.origin;
		if (origin === undefined) {
			return;
		}
		const onViolation = (event: SecurityPolicyViolationEvent) => {
			// blockedURI is a full URL or an origin. Parse it without a base,
			// so values such as "inline" never match.
			if (
				event.effectiveDirective !== "frame-src" ||
				event.disposition !== "enforce" ||
				parseURL(event.blockedURI)?.origin !== origin
			) {
				return;
			}
			// Coder's CSP always has this report-uri. A policy that a reverse
			// proxy adds does not.
			const byCoderPolicy = event.originalPolicy.includes(
				"/api/v2/csp/reports",
			);
			// Each blocking policy reports its own violation. Keep a reverse
			// proxy's block, because a reload cannot fix it.
			setBlock((prev) => ({
				src,
				origin,
				byCoderPolicy:
					byCoderPolicy && (prev?.src !== src || prev.byCoderPolicy),
			}));
		};
		document.addEventListener("securitypolicyviolation", onViolation);
		return () =>
			document.removeEventListener("securitypolicyviolation", onViolation);
	}, [src]);

	return block?.src === src ? block : undefined;
};
