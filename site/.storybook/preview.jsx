import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { createMemoryHistory } from "history"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { dark } from "../src/theme"
import "../src/theme/globalFonts"
import "../src/i18n"

const history = createMemoryHistory()

export const decorators = [
  (Story) => (
    <ThemeProvider theme={dark}>
      <CssBaseline />
      <Story />
    </ThemeProvider>
  ),
  (Story) => {
    return (
      <HistoryRouter history={history}>
        <Story />
      </HistoryRouter>
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
