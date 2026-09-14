import { createContext, type FC, type ReactNode, useContext } from "react";
import type { Theme } from "#/theme";

const ThemeContext = createContext<Theme | undefined>(undefined);

interface ThemeContextProviderProps {
	theme: Theme;
	children: ReactNode;
}

export const ThemeContextProvider: FC<ThemeContextProviderProps> = ({
	theme,
	children,
}) => {
	return (
		<ThemeContext.Provider value={theme}>{children}</ThemeContext.Provider>
	);
};

export const useTheme = (): Theme => {
	const theme = useContext(ThemeContext);
	if (theme === undefined) {
		throw new Error("useTheme must be used within a ThemeContextProvider");
	}
	return theme;
};
