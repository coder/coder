import CssBaseline from "@mui/material/CssBaseline";
import {
  StyledEngineProvider,
  ThemeProvider as MuiThemeProvider,
} from "@mui/material/styles";
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import { DecoratorHelpers } from "@storybook/addon-themes";
import { withRouter } from "storybook-addon-react-router-v6";
import { QueryClient, QueryClientProvider } from "react-query";
import { HelmetProvider } from "react-helmet-async";
import themes from "theme";
import "theme/globalFonts";

DecoratorHelpers.initializeThemeState(Object.keys(themes), "dark");

export const decorators = [
  (Story, context) => {
    const selectedTheme = DecoratorHelpers.pluckThemeFromContext(context);
    const { themeOverride } = DecoratorHelpers.useThemeParameters();
    const selected = themeOverride || selectedTheme || "dark";

    return (
      <StyledEngineProvider injectFirst>
        <MuiThemeProvider theme={themes[selected]}>
          <EmotionThemeProvider theme={themes[selected]}>
            <CssBaseline />
            <Story />
          </EmotionThemeProvider>
        </MuiThemeProvider>
      </StyledEngineProvider>
    );
  },
  withRouter,
  (Story) => {
    return (
      <HelmetProvider>
        <Story />
      </HelmetProvider>
    );
  },
  withQuery,
];

export const parameters = {
  options: {
    storySort: {
      method: "alphabetical",
      order: ["design", "pages", "components"],
      locales: "en-US",
    },
  },
  actions: {
    argTypesRegex: "^(on|handler)[A-Z].*",
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
    },
  },
};

function withQuery(Story, { parameters }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: Infinity,
        retry: false,
      },
    },
  });

  if (parameters.queries) {
    parameters.queries.forEach((query) => {
      queryClient.setQueryData(query.key, query.data);
    });
  }

  return (
    <QueryClientProvider client={queryClient}>
      <Story />
    </QueryClientProvider>
  );
}
