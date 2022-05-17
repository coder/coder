import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { createMemoryHistory } from "history"
import { addDecorator } from "node_modules/@storybook/react"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { dark } from "../src/theme"
import "../src/theme/globalFonts"

addDecorator((story) => (
  <ThemeProvider theme={dark}>
    <CssBaseline />
    {story()}
  </ThemeProvider>
))

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
