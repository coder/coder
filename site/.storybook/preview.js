import { CssBaseline } from "@material-ui/core"
import { createMuiTheme } from "@material-ui/core/styles"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { withThemes } from "@react-theming/storybook-addon"
import { createMemoryHistory } from "history"
import { addDecorator } from "node_modules/@storybook/react"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { dark, light } from "../src/theme"
import "../src/theme/global-fonts"

const providerFn = ({ theme, children }) => {
  const muiTheme = createMuiTheme(theme)
  return (
    <ThemeProvider theme={muiTheme}>
      <CssBaseline />
      {children}
    </ThemeProvider>
  )
}

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
