import "../src/index.css";
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import CssBaseline from "@mui/material/CssBaseline";
import {
	ThemeProvider as MuiThemeProvider,
	StyledEngineProvider,
} from "@mui/material/styles";
import { DecoratorHelpers } from "@storybook/addon-themes";
import isChromatic from "chromatic/isChromatic";
import { StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { withRouter } from "storybook-addon-remix-react-router";
import { TooltipProvider } from "../src/components/Tooltip/Tooltip";
import "theme/globalFonts";
import type { Decorator, Loader, Parameters } from "@storybook/react-vite";
import themes from "../src/theme";

DecoratorHelpers.initializeThemeState(Object.keys(themes), "dark");

export const parameters: Parameters = {
	options: {
		storySort: {
			method: "alphabetical",
			order: ["design", "pages", "modules", "components"],
			locales: "en-US",
		},
	},
	controls: {
		expanded: true,
		matchers: {
			color: /(background|color)$/i,
			date: /Date$/,
		},
	},
	viewport: {
		viewports: {
			ipad: {
				name: "iPad Mini",
				styles: {
					height: "1024px",
					width: "768px",
				},
				type: "tablet",
			},
			iphone12: {
				name: "iPhone 12",
				styles: {
					height: "844px",
					width: "390px",
				},
				type: "mobile",
			},
			terminal: {
				name: "Terminal",
				styles: {
					height: "400",
					width: "400",
				},
			},
		},
	},
};

const withQuery: Decorator = (Story, { parameters }) => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: {
				staleTime: Number.POSITIVE_INFINITY,
				retry: false,
			},
		},
	});

	if (parameters.queries) {
		for (const query of parameters.queries) {
			queryClient.setQueryData(query.key, query.data);
		}
	}

	return (
		<QueryClientProvider client={queryClient}>
			<Story />
		</QueryClientProvider>
	);
};

const withTheme: Decorator = (Story, context) => {
	const selectedTheme = DecoratorHelpers.pluckThemeFromContext(context);
	const { themeOverride } = DecoratorHelpers.useThemeParameters();
	const selected = themeOverride || selectedTheme || "dark";

	// Ensure the correct theme is applied to Tailwind CSS classes by adding the
	// theme to the HTML class list. This approach is necessary because Tailwind
	// CSS relies on class names to apply styles, and dynamically changing themes
	// requires updating the class list accordingly.
	document.querySelector("html")?.setAttribute("class", selected);

	return (
		<StrictMode>
			<StyledEngineProvider injectFirst>
				<MuiThemeProvider theme={themes[selected]}>
					<EmotionThemeProvider theme={themes[selected]}>
						<TooltipProvider delayDuration={100}>
							<CssBaseline />
							<Story />
						</TooltipProvider>
					</EmotionThemeProvider>
				</MuiThemeProvider>
			</StyledEngineProvider>
		</StrictMode>
	);
};

export const decorators: Decorator[] = [withRouter, withQuery, withTheme];

// Try to fix storybook rendering fonts inconsistently
// https://www.chromatic.com/docs/font-loading/#solution-c-check-fonts-have-loaded-in-a-loader
const fontLoader = async () => ({
	fonts: await document.fonts.ready,
});

export const loaders: Loader[] =
	isChromatic() && document.fonts ? [fontLoader] : [];
