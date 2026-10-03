import { useMonaco } from "@monaco-editor/react";
import { useEffect, useState } from "react";
import { useTheme } from "#/theme/context";

export const useCoderTheme = (): { isLoading: boolean; name: string } => {
	const [isLoading, setIsLoading] = useState(true);
	const monaco = useMonaco();
	const theme = useTheme();
	const name = "coder";

	useEffect(() => {
		if (monaco) {
			monaco.editor.defineTheme(name, theme.monaco);
			setIsLoading(false);
		}
	}, [monaco, theme]);

	return {
		isLoading,
		name,
	};
};
