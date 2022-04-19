import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { withThemes } from "@react-theming/storybook-addon"
import { createMemoryHistory } from "history"
import { addDecorator } from "node_modules/@storybook/react"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { dark, light } from "../src/theme"
import "../src/theme/globalFonts"

const providerFn = ({ children, theme }) => (
  <ThemeProvider theme={theme}>
    <CssBaseline />
    {children}
  </ThemeProvider>
)

addDecorator(withThemes(null, [light, dark], { providerFn }))

const history = createMemoryHistory()

const routerDecorator = (Story) => {
  return (
    <HistoryRouter history={history}>
      <Story />
    </HistoryRouter>
  )
}

addDecorator(routerDecorator)

export const parameters = {
  actions: {
    argTypesRegex: "^on[A-Z].*",
    argTypesRegex: "^handle[A-Z].*",
  },
  controls: {
    expanded: true,
    matchers: {
      color: /(background|color)$/i,
      date: /Date$/,
    },
  },
}
