import type { UserAppearanceSettings } from "#/api/typesGenerated";
import { DEFAULT_TERMINAL_FONT, terminalFonts } from "#/theme/constants";

export function getTerminalConfig(
	appearance: UserAppearanceSettings | undefined,
	proxyPathAppURL: string | undefined,
) {
	return {
		baseUrl:
			process.env.NODE_ENV !== "development" ? proxyPathAppURL : undefined,
		fontFamily:
			terminalFonts[appearance?.terminal_font || DEFAULT_TERMINAL_FONT],
	};
}
