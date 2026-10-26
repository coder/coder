import { useEffect } from "react";
import { useQuery } from "react-query";
import { appearanceSettings } from "#/api/queries/users";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import themes, { baseModeFor, CONCRETE_THEMES, type Theme } from "#/theme";
import { AppearanceProvider } from "#/theme/appearance";
import { ThemeContextProvider } from "#/theme/context";
import {
	migrateLegacyPreference,
	resolveActiveThemeName,
} from "#/theme/themeMode";
import { usePreferredColorScheme } from "#/theme/usePreferredColorScheme";

export const ThemeProvider: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const preferredColorScheme = usePreferredColorScheme();

	const settings =
		appearanceSettingsQuery.data ?? metadata.userAppearance?.value ?? {};
	const state = migrateLegacyPreference(settings);
	const concreteName = resolveActiveThemeName(state, preferredColorScheme);

	useEffect(() => {
		const root = document.documentElement;
		// Embedded pages manage theme independently.
		if (root.dataset.embedTheme) {
			return;
		}
		root.classList.add(concreteName);
		root.classList.add(baseModeFor(concreteName));

		// Installed PWAs and mobile browsers paint their title bar or toolbar
		// with theme-color. Reading the body background after the classes
		// apply gives the surface this theme actually renders.
		const themeColor = getComputedStyle(document.body).backgroundColor;
		for (const meta of document.querySelectorAll<HTMLMetaElement>(
			'meta[name="theme-color"]',
		)) {
			meta.content = themeColor;
		}

		return () => {
			if (!root.dataset.embedTheme) {
				root.classList.remove(...CONCRETE_THEMES);
			}
		};
	}, [concreteName]);

	const theme = themes[concreteName];

	return <ThemeOverride theme={theme}>{children}</ThemeOverride>;
};

type ThemeOverrideProps = {
	theme: Theme;
	children?: React.ReactNode;
};

export const ThemeOverride: React.FC<ThemeOverrideProps> = ({
	theme,
	children,
}) => {
	return (
		<ThemeContextProvider theme={theme}>
			<AppearanceProvider externalImages={theme.externalImages}>
				{children}
			</AppearanceProvider>
		</ThemeContextProvider>
	);
};
