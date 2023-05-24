import CssBaseline from "@mui/material/CssBaseline"
import { StyledEngineProvider, ThemeProvider } from "@mui/material/styles"
import { createMemoryHistory } from "history"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { HelmetProvider } from "react-helmet-async"
import { dark } from "../src/theme"
import "../src/theme/globalFonts"
import "../src/i18n"

const history = createMemoryHistory()

export const decorators = [
  (Story) => (
    <StyledEngineProvider injectFirst>
      <ThemeProvider theme={dark}>
        <CssBaseline />
        <Story />
      </ThemeProvider>
    </StyledEngineProvider>
  ),
  (Story) => {
    return (
      <HistoryRouter history={history}>
        <Story />
      </HistoryRouter>
    )
  },
  (Story) => {
    return (
      <HelmetProvider>
        <Story />
      </HelmetProvider>
    )
  },
]

export const parameters = {
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
}
