import "../src/index.css";
import "../src/theme/globalFonts";
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import CssBaseline from "@mui/material/CssBaseline";
import {
	ThemeProvider as MuiThemeProvider,
	StyledEngineProvider,
} from "@mui/material/styles";
import { DecoratorHelpers } from "@storybook/addon-themes";
import type { Decorator, Loader, Parameters } from "@storybook/react-vite";
import isChromatic from "chromatic/isChromatic";
import { StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { withRouter } from "storybook-addon-remix-react-router";
import { TooltipProvider } from "../src/components/Tooltip/Tooltip";
import themes, { baseModeFor, isConcreteThemeName } from "../src/theme";

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
		options: {
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
			// Approximates a 1440x900 desktop viewed at 200% browser zoom,
			// which collapses the CSS viewport to 720x450. Used by stories
			// that verify the desktop layout still renders at common zoom
			// levels. Below the Tailwind sm: breakpoint (640 px), the
			// AgentsPage collapses into the mobile stack, so 720 px stays
			// on the desktop branch.
			desktopZoom200: {
				name: "Desktop @ 200% zoom (720x450)",
				styles: {
					height: "450px",
					width: "720px",
				},
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
				refetchInterval: false,
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
	const { themeOverride } = DecoratorHelpers.useThemeParameters() ?? {};
	const selected = themeOverride || selectedTheme || "dark";
	const concreteName = isConcreteThemeName(selected) ? selected : "dark";
	const htmlClassName = `${baseModeFor(concreteName)} ${concreteName}`;
	// Ensure the correct theme is applied to Tailwind CSS classes by adding the
	// concrete theme and base mode to the HTML class list. This mirrors the
	// production ThemeProvider so Tailwind's selector-based `dark:` variant keeps
	// working in Storybook when a dark colorblind variant is active.
	document.querySelector("html")?.setAttribute("class", htmlClassName);

	return (
		<StrictMode>
			<StyledEngineProvider injectFirst>
				<MuiThemeProvider theme={themes[concreteName]}>
					<EmotionThemeProvider theme={themes[concreteName]}>
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
