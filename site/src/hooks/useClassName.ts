import { css } from "@emotion/css";
import { type Theme, useTheme } from "@emotion/react";
import { type DependencyList, useMemo } from "react";

type ClassName = (cssFn: typeof css, theme: Theme) => string;

/**
 * @deprecated This hook was used as an escape hatch to generate class names
 * using emotion when no other styling method would work. There is no valid new
 * usage of this hook. Use Tailwind classes instead.
 */
export function useClassName(styles: ClassName, deps: DependencyList): string {
	const theme = useTheme();
	// biome-ignore lint/correctness/useExhaustiveDependencies: depends on deps
	const className = useMemo(() => {
		return styles(css, theme);
	}, [...deps, theme]);

	return className;
}
