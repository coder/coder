import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { createMemoryHistory } from "history"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { dark } from "../src/theme"
import "../src/theme/globalFonts"
import "../src/i18n"
import React from "react"
import { Preview } from '@storybook/react';


const themeProviderDecorator = (story) => (
  <ThemeProvider theme={dark}>
    <CssBaseline />
    {story()}
  </ThemeProvider>
)

const history = createMemoryHistory()

const routerDecorator = (Story) => {
  return (
    <HistoryRouter history={history}>
      <Story />
    </HistoryRouter>
  )
}

const preview: Preview = {
  decorators: [themeProviderDecorator, routerDecorator],
  parameters: {
    actions: {
      argTypesRegex: "(^on[A-Z].*)?(^handle[A-Z].*)",
    },
    controls: {
      expanded: true,
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/,
      },
    },
  },
}

export default preview
