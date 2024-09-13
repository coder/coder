// @ts-check
/**
 * @file Defines the main configuration file for all of our Storybook tests.
 * This file must be a JSX/JS file, but we can at least add some type safety via
 * the ts-check directive.
 * @see {@link https://storybook.js.org/docs/configure#configure-story-rendering}
 *
 * @typedef {import("react").ReactElement} ReactElement
 * @typedef {import("react").PropsWithChildren} PropsWithChildren
 * @typedef {import("react").FC<PropsWithChildren>} FC
 *
 * @typedef {import("@storybook/react").StoryContext} StoryContext
 * @typedef {import("@storybook/react").Preview} Preview
 *
 * @typedef {(Story: FC, Context: StoryContext) => React.JSX.Element} Decorator A
 * Storybook decorator function used to inject baseline data dependencies into
 * our React components during testing.
 */
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import CssBaseline from "@mui/material/CssBaseline";
import {
	ThemeProvider as MuiThemeProvider,
	StyledEngineProvider,
} from "@mui/material/styles";
import { DecoratorHelpers } from "@storybook/addon-themes";
import isChromatic from "chromatic/isChromatic";
import React, { StrictMode } from "react";
import { HelmetProvider } from "react-helmet-async";
import { parseQueryArgs, QueryClient, QueryClientProvider } from "react-query";
import { withRouter } from "storybook-addon-remix-react-router";
import "theme/globalFonts";
import themes from "../src/theme";

DecoratorHelpers.initializeThemeState(Object.keys(themes), "dark");

/** @type {readonly Decorator[]} */
export const decorators = [withRouter, withQuery, withHelmet, withTheme];

/** @type {Preview["parameters"]} */
export const parameters = {
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

/**
 * There's a mismatch on the React Helmet return type that causes issues when
 * mounting the component in JS files only. Have to do type assertion, which is
 * especially ugly in JSDoc
 */
const SafeHelmetProvider = /** @type {FC} */ (
	/** @type {unknown} */ (HelmetProvider)
);

/** @type {Decorator} */
function withHelmet(Story) {
	return (
		<SafeHelmetProvider>
			<Story />
		</SafeHelmetProvider>
	);
}

/** @type {Decorator} */
function withQuery(Story, { parameters }) {
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
			if (query.data instanceof Error) {
				// This is copied from setQueryData() but sets the error.
				const cache = queryClient.getQueryCache();
				const parsedOptions = parseQueryArgs(query.key);
				const defaultedOptions = queryClient.defaultQueryOptions(parsedOptions);
				const cachedQuery = cache.build(queryClient, defaultedOptions);
				// Set manual data so react-query will not try to refetch.
				cachedQuery.setData(undefined, { manual: true });
				cachedQuery.setState({ error: query.data });
			} else {
				queryClient.setQueryData(query.key, query.data);
			}
		}
	}

	return (
		<QueryClientProvider client={queryClient}>
			<Story />
		</QueryClientProvider>
	);
}

/** @type {Decorator} */
function withTheme(Story, context) {
	const selectedTheme = DecoratorHelpers.pluckThemeFromContext(context);
	const { themeOverride } = DecoratorHelpers.useThemeParameters();
	const selected = themeOverride || selectedTheme || "dark";

	return (
		<StrictMode>
			<StyledEngineProvider injectFirst>
				<MuiThemeProvider theme={themes[selected]}>
					<EmotionThemeProvider theme={themes[selected]}>
						<CssBaseline />
						<Story />
					</EmotionThemeProvider>
				</MuiThemeProvider>
			</StyledEngineProvider>
		</StrictMode>
	);
}

// Try to fix storybook rendering fonts inconsistently
// https://www.chromatic.com/docs/font-loading/#solution-c-check-fonts-have-loaded-in-a-loader
const fontLoader = async () => ({
	fonts: await document.fonts.ready,
});

export const loaders = isChromatic() && document.fonts ? [fontLoader] : [];
