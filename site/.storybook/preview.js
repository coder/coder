import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { withThemes } from "@react-theming/storybook-addon"
import { light, dark } from "../theme"
import { addDecorator } from "node_modules/@storybook/react"

addDecorator(withThemes(ThemeProvider, [light, dark]))

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
