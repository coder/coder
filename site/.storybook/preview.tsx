import "../src/index.css";
import "../src/theme/globalFonts";
import { isPixel } from "@coder/pixel-storybook/storyapi";
import { DecoratorHelpers } from "@storybook/addon-themes";
import type { Decorator, Parameters } from "@storybook/react-vite";
import { MotionConfig, MotionGlobalConfig } from "motion/react";
import { StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { withRouter } from "storybook-addon-remix-react-router";
import { TooltipProvider } from "../src/components/Tooltip/Tooltip";
import themes, { baseModeFor, isConcreteThemeName } from "../src/theme";
import { AppearanceProvider } from "../src/theme/appearance";
import { ThemeContextProvider } from "../src/theme/context";

DecoratorHelpers.initializeThemeState(Object.keys(themes), "dark");

MotionGlobalConfig.skipAnimations = isPixel();

// Two Radix modal-layer behaviors race play functions under pixel, so both
// are neutralized there only; vitest, Storybook dev, and the app keep their
// animations and layer behavior.
// 1. Exit-animating layers stay mounted (with body pointer-events locked and
//    background aria-hidden) until their CSS animation ends, so animations
//    are disabled outright and open/close becomes synchronous. Near-zero
//    durations are not enough: the cleanup then lands one frame after a
//    play's next query.
// 2. Opening a modal locks body pointer-events in an effect but re-renders
//    the dialog content with inline pointer-events auto one commit later; a
//    play's first interaction can land inside that window, so dialog
//    surfaces are pre-granted pointer-events auto.
if (isPixel()) {
	const style = document.createElement("style");
	style.textContent = `
		*, *::before, *::after { animation: none !important; transition: none !important; }
		[role="dialog"], [role="alertdialog"] { pointer-events: auto !important; }
	`;
	document.head.appendChild(style);
}

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

const withTheme: Decorator = function WithTheme(Story, context) {
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
			<ThemeContextProvider theme={themes[concreteName]}>
				<AppearanceProvider
					externalImages={themes[concreteName].externalImages}
				>
					<TooltipProvider delayDuration={100}>
						<Story />
					</TooltipProvider>
				</AppearanceProvider>
			</ThemeContextProvider>
		</StrictMode>
	);
};

const withSkipAnimations: Decorator = (Story) => (
	<MotionConfig skipAnimations={isPixel()}>
		<Story />
	</MotionConfig>
);

export const decorators: Decorator[] = [
	withRouter,
	withQuery,
	withTheme,
	withSkipAnimations,
];
