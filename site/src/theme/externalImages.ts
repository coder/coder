import type { CSSProperties } from "react";

export interface ExternalImageModeStyles {
	/**
	 * monochrome icons will be flattened to a neutral, theme-appropriate color.
	 * eg. white, light gray, dark gray, black
	 */
	monochrome?: CSSProperties;
	/**
	 * @default
	 * fullcolor icons should look their best of any background, with distinct colors
	 * and good contrast. This is the default, and won't alter the image.
	 */
	fullcolor?: CSSProperties;
	/**
	 * whiteWithColor is useful for icons that are primarily white, or contain white text,
	 * which are hard to see or look incorrect on light backgrounds. This setting will apply
	 * a color-respecting inversion filter to turn white into black when appropriate to
	 * improve contrast.
	 * You can also specify a `brightness` level if your icon still doesn't look quite right.
	 * eg. /icon/aws.svg?blackWithColor&brightness=1.5
	 */
	whiteWithColor?: CSSProperties;
	/**
	 * blackWithColor is useful for icons that are primarily black, or contain black text,
	 * which are hard to see or look incorrect on dark backgrounds. This setting will apply
	 * a color-respecting inversion filter to turn black into white when appropriate to
	 * improve contrast.
	 * You can also specify a `brightness` level if your icon still doesn't look quite right.
	 * eg. /icon/aws.svg?blackWithColor&brightness=1.5
	 */
	blackWithColor?: CSSProperties;
}

export const forDarkThemes: ExternalImageModeStyles = {
	// brighten icons a little to make sure they have good contrast with the background
	monochrome: { filter: "grayscale(100%) contrast(0%) brightness(250%)" },
	// do nothing to full-color icons
	fullcolor: undefined,
	// white on a dark background ✅
	whiteWithColor: undefined,
	// black on a dark background 🆘: invert, and then correct colors
	blackWithColor: { filter: "invert(1) hue-rotate(180deg)" },
};

export const forLightThemes: ExternalImageModeStyles = {
	// darken icons a little to make sure they have good contrast with the background
	monochrome: { filter: "grayscale(100%) contrast(0%) brightness(70%)" },
	// do nothing to full-color icons
	fullcolor: undefined,
	// black on a dark background 🆘: invert, and then correct colors
	whiteWithColor: { filter: "invert(1) hue-rotate(180deg)" },
	// black on a light background ✅
	blackWithColor: undefined,
};

// multiplier matches the beginning of the string (^), a number, optionally followed
// followed by a decimal portion, optionally followed by a percent symbol, and the
// end of the string ($).
const multiplier = /^\d+(\.\d+)?%?$/;

/**
 * Used with `whiteWithColor` and `blackWithColor` to allow for finer tuning
 */
const parseInvertFilterParameters = (
	params: URLSearchParams,
	baseStyles?: CSSProperties,
) => {
	// Only apply additional styles if the current theme supports this mode
	if (!baseStyles) {
		return;
	}

	let extraStyles: CSSProperties | undefined;

	const brightness = params.get("brightness") ?? "";
	if (multiplier.test(brightness)) {
		let filter = baseStyles.filter ?? "";
		filter += ` brightness(${brightness})`;
		extraStyles = { ...extraStyles, filter };
	}

	if (!extraStyles) {
		return baseStyles;
	}

	return {
		...baseStyles,
		...extraStyles,
	};
};

export function parseImageParameters(
	modes: ExternalImageModeStyles,
	searchString: string,
): CSSProperties | undefined {
	const params = new URLSearchParams(searchString);

	let styles: CSSProperties | undefined = modes.fullcolor;

	if (params.has("monochrome")) {
		styles = modes.monochrome;
	} else if (params.has("whiteWithColor")) {
		styles = parseInvertFilterParameters(params, modes.whiteWithColor);
	} else if (params.has("blackWithColor")) {
		styles = parseInvertFilterParameters(params, modes.blackWithColor);
	}

	return styles;
}

export function getExternalImageStylesFromUrl(
	modes: ExternalImageModeStyles,
	urlString?: string,
) {
	if (!urlString) {
		return undefined;
	}

	const url = new URL(urlString, location.origin);

	if (url.search) {
		return parseImageParameters(modes, url.search);
	}

	if (
		url.origin === location.origin &&
		defaultParametersForBuiltinIcons.has(url.pathname)
	) {
		return parseImageParameters(
			modes,
			defaultParametersForBuiltinIcons.get(url.pathname) as string,
		);
	}

	return undefined;
}

/**
 * defaultModeForBuiltinIcons contains modes for all of our built-in icons that
 * don't look their best in all of our themes with the default fullcolor mode.
 */
export const defaultParametersForBuiltinIcons = new Map<string, string>([
	["/icon/apple-black.svg", "monochrome"],
	["/icon/auggie.svg", "monochrome"],
	["/icon/anthropic.svg", "monochrome"],
	["/icon/auto-dev-server.svg", "monochrome"],
	["/icon/aws-monochrome.svg", "monochrome"],
	["/icon/aws.png", "whiteWithColor&brightness=1.5"],
	["/icon/aws.svg", "whiteWithColor&brightness=1.5"],
	["/icon/coder.svg", "monochrome"],
	["/icon/container.svg", "monochrome"],
	["/icon/copyparty.svg", "blackWithColor"],
	["/icon/database.svg", "monochrome"],
	["/icon/devcontainers.svg", "monochrome"],
	["/icon/docker-white.svg", "monochrome"],
	["/icon/folder.svg", "monochrome"],
	["/icon/gemini-monochrome.svg", "monochrome"],
	["/icon/github-copilot.svg", "whiteWithColor"],
	["/icon/github.svg", "monochrome"],
	["/icon/image.svg", "monochrome"],
	["/icon/jupyter.svg", "blackWithColor"],
	["/icon/kasmvnc.svg", "whiteWithColor"],
	["/icon/kilo-code.svg", "blackWithColor"],
	["/icon/kiro.svg", "whiteWithColor"],
	["/icon/memory.svg", "monochrome"],
	["/icon/mux.svg", "monochrome"],
	["/icon/nexus-repository.svg", "blackWithColor"],
	["/icon/okta.svg", "monochrome"],
	["/icon/openai-codex.svg", "monochrome"],
	["/icon/openai.svg", "monochrome"],
	["/icon/openwebui.svg", "monochrome"],
	["/icon/perplexica.svg", "monochrome"],
	["/icon/roo-code.svg", "whiteWithColor"],
	["/icon/rust.svg", "monochrome"],
	["/icon/tasks.svg", "monochrome"],
	["/icon/terminal.svg", "monochrome"],
	["/icon/widgets.svg", "monochrome"],
	["/icon/windsurf.svg", "monochrome"],
	["/icon/zed.svg", "monochrome"],
]);
