import CssBaseline from "@mui/material/CssBaseline";
import {
  StyledEngineProvider,
  ThemeProvider as MuiThemeProvider,
} from "@mui/material/styles";
import { ThemeProvider as EmotionThemeProvider } from "@emotion/react";
import { withRouter } from "storybook-addon-react-router-v6";
import { HelmetProvider } from "react-helmet-async";
import theme from "theme";
import colors from "theme/tailwind";
import "theme/globalFonts";
import { QueryClient, QueryClientProvider } from "react-query";

export const decorators = [
  (Story) => (
    <StyledEngineProvider injectFirst>
      <MuiThemeProvider theme={theme.dark}>
        <EmotionThemeProvider theme={theme.dark}>
          <CssBaseline />
          <Story />
        </EmotionThemeProvider>
      </MuiThemeProvider>
    </StyledEngineProvider>
  ),
  withRouter,
  (Story) => {
    return (
      <HelmetProvider>
        <Story />
      </HelmetProvider>
    );
  },
  (Story) => {
    return (
      <QueryClientProvider
        client={
          new QueryClient({
            defaultOptions: {
              queries: {
                staleTime: Infinity,
              },
            },
          })
        }
      >
        <Story />
      </QueryClientProvider>
    );
  },
];

export const parameters = {
  backgrounds: {
    default: "dark",
    values: [
      {
        name: "dark",
        value: colors.gray[950],
      },
      {
        name: "light",
        value: colors.gray[50],
      },
    ],
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
};
