import type {
	DeploymentConfig,
	UserAppearanceSettings,
} from "#/api/typesGenerated";
import { DEFAULT_TERMINAL_FONT, terminalFonts } from "#/theme/constants";

export function getTerminalConfig(
	config: DeploymentConfig | undefined,
	appearance: UserAppearanceSettings | undefined,
	proxyPathAppURL: string | undefined,
) {
	return {
		renderer: config?.config?.web_terminal_renderer,
		baseUrl:
			process.env.NODE_ENV !== "development" ? proxyPathAppURL : undefined,
		fontFamily:
			terminalFonts[appearance?.terminal_font || DEFAULT_TERMINAL_FONT],
	};
}
