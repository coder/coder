import {
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useMemo,
} from "react";
import {
	type ExternalImageModeStyles,
	forDarkThemes,
} from "#/theme/externalImages";

/**
 * Publishes appearance-related values derived from the active site theme so
 * components can consume them without depending on Emotion's `useTheme`. The
 * values originate from the site theme and are provided by `ThemeOverride`.
 */
interface Appearance {
	externalImages: ExternalImageModeStyles;
}

const AppearanceContext = createContext<Appearance>({
	// Defaults to `forDarkThemes` to match `DEFAULT_THEME`.
	externalImages: forDarkThemes,
});

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
	return useContext(AppearanceContext);
};
