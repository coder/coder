import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { withThemes } from "@react-theming/storybook-addon"
import { light, dark } from "../src/theme"
import { addDecorator } from "node_modules/@storybook/react"
import { createMemoryHistory } from "history"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import "../src/theme/global-fonts"

addDecorator(withThemes(ThemeProvider, [light, dark]))

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
