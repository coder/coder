import {
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useMemo,
} from "react";
import type { ExternalImageModeStyles } from "#/theme/externalImages";

/**
 * Publishes *user* appearance values derived from the active site theme so
 * components can consume them without depending on Emotion's `useTheme`.
 *
 * This is the client-side, theme-derived appearance surface (for example, how
 * external images should be tinted for the active theme). It is intentionally
 * distinct from the user `appearanceSettings` query (theme, terminal font) and
 * from the deployment-level `AppearanceConfig` (application name, logo, service
 * banners) that admins configure.
 *
 * Values are provided by the surrounding `AppearanceProvider` (see
 * `ThemeOverride` and the Storybook preview decorator).
 */
interface Appearance {
	externalImages: ExternalImageModeStyles;
}

const AppearanceContext = createContext<Appearance | undefined>(undefined);

interface AppearanceProviderProps {
	externalImages: ExternalImageModeStyles;
	children: ReactNode;
}

export const AppearanceProvider: FC<AppearanceProviderProps> = ({
	externalImages,
	children,
}) => {
	const value = useMemo<Appearance>(
		() => ({ externalImages }),
		[externalImages],
	);

	return (
		<AppearanceContext.Provider value={value}>
			{children}
		</AppearanceContext.Provider>
	);
};

export const useAppearance = (): Appearance => {
	const appearance = useContext(AppearanceContext);
	if (appearance === undefined) {
		throw new Error("useAppearance must be used within an AppearanceProvider");
	}
	return appearance;
};
