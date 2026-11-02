import {
	type ILink,
	type ILinkProvider,
	OSC8LinkProvider,
	type Terminal,
	UrlRegexProvider,
} from "ghostty-web";

/**
 * Detects the same OSC 8 hyperlinks and plain URLs as the terminal's
 * built-in providers, but opens them through `openLink` so port-forwarded
 * URLs are rewritten. Links are registered with the same ranges as the
 * built-in ones, so they take precedence.
 */
export const createLinkProvider = (
	terminal: Terminal,
	openLink: (uri: string) => void,
): ILinkProvider => {
	const providers: ILinkProvider[] = [
		new OSC8LinkProvider(terminal),
		new UrlRegexProvider(terminal),
	];
	return {
		provideLinks(y, callback) {
			const links: ILink[] = [];
			let pending = providers.length;
			for (const provider of providers) {
				provider.provideLinks(y, (found) => {
					for (const link of found ?? []) {
						links.push({
							...link,
							activate: () => {
								// A drag that ends on a link selects text; it should not
								// also open the link.
								if (terminal.hasSelection()) {
									return;
								}
								openLink(link.text);
							},
						});
					}
					pending--;
					if (pending === 0) {
						callback(links.length > 0 ? links : undefined);
					}
				});
			}
		},
		dispose() {
			for (const provider of providers) {
				provider.dispose?.();
			}
		},
	};
};
