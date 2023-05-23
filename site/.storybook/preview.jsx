import "../src/theme/globalFonts"
import "../src/i18n"
import { initialize, mswDecorator } from "msw-storybook-addon"
import { handlers } from "../src/testHelpers/handlers"
import { AppProviders } from "../src/app"
import { MemoryRouter } from "react-router-dom"

// Initialize MSW

initialize({
  onUnhandledRequest: (req, print) => {
    if (req.url.pathname.startsWith("/api")) {
      print.warning()
    }
  },
})

export const decorators = [
  (Story) => (
    <AppProviders>
      <Story />
    </AppProviders>
  ),
  (Story, { parameters }) => {
    if (parameters.router === false) {
      return <Story />
    }
    return (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    )
  },
  mswDecorator,
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
  msw: {
    handlers,
  },
  router: true,
}
