# Frontend

This is a guide to help the Coder community and also Coder members contribute to
our UI. It is ongoing work but we hope it provides some useful information to
get started. If you have any questions or need help, please send us a message on
our [Discord server](https://discord.com/invite/coder). We'll be happy to help
you.

## Running the UI

You can run the UI and access the dashboard in two ways:

- Build the UI pointing to an external Coder server:
  `CODER_HOST=https://mycoder.com pnpm dev` inside of the `site` folder. This is
  helpful when you are building something in the UI and already have the data on
  your deployed server.
- Build the entire Coder server + UI locally: `./scripts/develop.sh` in the root
  folder. It is useful when you have to contribute with features that are not
  deployed yet or when you have to work on both, frontend and backend.

In both cases, you can access the dashboard on `http://localhost:8080`. If you
are running the `./scripts/develop.sh` you can log in using the default
credentials: `admin@coder.com` and `SomeSecurePassword!`.

## Tech Stack

All our dependencies are described in `site/package.json` but here are the most
important ones:

- [React](https://reactjs.org/) as framework
- [Typescript](https://www.typescriptlang.org/) to keep our sanity
- [Vite](https://vitejs.dev/) to build the project
- [Material V5](https://mui.com/material-ui/getting-started/) for UI components
- [react-router](https://reactrouter.com/en/main) for routing
- [TanStack Query v4](https://tanstack.com/query/v4/docs/react/overview) for
  fetching data
- [XState](https://xstate.js.org/docs/) for handling complex state flows
- [axios](https://github.com/axios/axios) as fetching lib
- [Playwright](https://playwright.dev/) for E2E testing
- [Jest](https://jestjs.io/) for integration testing
- [Storybook](https://storybook.js.org/) and
  [Chromatic](https://www.chromatic.com/) for visual testing
- [PNPM](https://pnpm.io/) as package manager

## Structure

All the code related to the UI is inside the `site` folder and we defined a few
conventions to help people to navigate through it.

- **e2e** - E2E tests
- **src** - Source code
  - **mocks** - [Manual mocks](https://jestjs.io/docs/manual-mocks) used by Jest
  - **@types** - Custom types for dependencies that don't have defined types
  - **api** - API code as function calls and types
  - **components** - UI components
  - **hooks** - Hooks that can be used across the application
  - **i18n** - Translations
  - **pages** - Page components
  - **testHelpers** - Helper functions to help with integration tests
  - **util** - Helper functions that can be used across the application
  - **xServices** - XState machines used to fetch data and handle complex
    scenarios
- **static** - UI static assets like images, fonts, icons, etc

## Routing

We use [react-router](https://reactrouter.com/en/main) as our routing engine and
adding a new route is very easy. If the new route needs to be authenticated, put
it under the `<RequireAuth>` route and if it needs to live inside of the
dashboard, put it under the `<DashboardLayout>` route.

The `RequireAuth` component handles all the authentication logic for the routes
and the `DashboardLayout` wraps the route adding a navbar and passing down
common dashboard data.

## Pages

Pages are the top-level components of the app. The page component lives under
the `src/pages` folder and each page should have its own folder so we can better
group the views, tests, utility functions and so on. We use a structure where
the page component is responsible for fetching all the data and passing it down
to the view. We explain this decision a bit better in the next section.

> ℹ️ Code that is only related to the page should live inside of the page folder
> but if at some point it is used in other pages or components, you should
> consider moving it to the `src` level in the `utils`, `hooks` or `components`
> folder.

### States

A page usually has at least three states: **loading**, **ready** or **success**,
and **error** so remember to always handle these scenarios while you are coding
a page. We also encourage you to add visual testing for these three states using
the `*.stories.ts` file.

## Fetching data

We use
[TanStack Query v4](https://tanstack.com/query/v4/docs/react/overview)(previously
known as react-query) to fetch data from the API. We also use
[XState](https://xstate.js.org/docs/) to handle complex flows with multiple
states and transitions.

> ℹ️ We recently changed how we are going to fetch data from the server so you
> will see a lot of fetches being made using XState machines but feel free to
> refactor it if you are already touching those files.

### Where to fetch data

Finding the right place to fetch data in React apps is the one million dollar
question but we decided to make it only in the page components and pass the
props down to the views. This makes it easier to find where data is being loaded
and easy to test using Storybook. So you will see components like `UsersPage`
and `UsersPageView`.

### API

We are using [axios](https://github.com/axios/axios) as our fetching library and
writing the API functions in the `site/src/api/api.ts` files. We also have
auto-generated types from our Go server on `site/src/api/typesGenerated.ts`.
Usually, every endpoint has its own ` Request` and `Response` types but
sometimes you need to pass extra parameters to make the call like the example
below:

```ts
export const getAgentListeningPorts = async (
  agentID: string,
): Promise<TypesGen.ListeningPortsResponse> => {
  const response = await axios.get(
    `/api/v2/workspaceagents/${agentID}/listening-ports`,
  );
  return response.data;
};
```

Sometimes, a FE operation can have multiple API calls so it is ok to wrap it as
a single function.

```ts
export const updateWorkspaceVersion = async (
  workspace: TypesGen.Workspace,
): Promise<TypesGen.WorkspaceBuild> => {
  const template = await getTemplate(workspace.template_id);
  return startWorkspace(workspace.id, template.active_version_id);
};
```

If you need more granular errors or control, you may should consider keep them
separated and use XState for that.

## Components

We are using [Material V4](https://v4.mui.com/) in our UI and we don't have any
short-term plans to update or even replace it. It still provides good value for
us and changing it would cost too much work which is not valuable right now but
of course, it can change in the future.

### Structure

Each component gets its own folder. Make sure you add a test and Storybook
stories for the component as well. By keeping these tidy, the codebase will
remain easy to navigate, healthy and maintainable for all contributors.

### Accessibility

We strive to keep our UI accessible. When using colors, avoid adding new
elements with low color contrast. Always use labels on inputs, not just
placeholders. These are important for screen-readers.

### Should I create a new component?

As with most things in the world, it depends. If you are creating a new
component to encapsulate some UI abstraction like `UsersTable` it is ok but you
should always try to use the base components that are provided by the library or
from the codebase so I recommend you to always do a quick search before creating
a custom primitive component like dialogs, popovers, buttons, etc.

## Testing

We use three types of testing in our app: **E2E**, **Integration** and **Visual
Testing**.

### E2E (end-to-end)

Are useful to test complete flows like "Create a user", "Import template", etc.
For this one, we use [Playwright](https://playwright.dev/). If you only need to
test if the page is being rendered correctly, you should probably consider using
the **Visual Testing** approach.

> ℹ️ For scenarios where you need to be authenticated, you can use
> `test.use({ storageState: getStatePath("authState") })`.

### Integration

Test user interactions like "Click in a button shows a dialog", "Submit the form
sends the correct data", etc. For this, we use [Jest](https://jestjs.io/) and
[react-testing-library](https://testing-library.com/docs/react-testing-library/intro/).
If the test involves routing checks like redirects or maybe checking the info on
another page, you should probably consider using the **E2E** approach.

### Visual testing

Test components without user interaction like testing if a page view is rendered
correctly depending on some parameters, if the button is showing a spinner if
the `loading` props are passing, etc. This should always be your first option
since it is way easier to maintain. For this, we use
[Storybook](https://storybook.js.org/) and
[Chromatic](https://www.chromatic.com/).

### What should I test?

Choosing what to test is not always easy since there are a lot of flows and a
lot of things can happen but these are a few indicators that can help you with
that:

- Things that can block the user
- Reported bugs
- Regression issues

### Tests getting too slow

A few times you can notice tests can take a very long time to get done.
Sometimes it is because the test itself is complex and runs a lot of stuff, and
sometimes it is because of how we are querying things. In the next section, we
are going to talk more about them.

#### Using `ByRole` queries

One thing we figured out that was slowing down our tests was the use of `ByRole`
queries because of how it calculates the role attribute for every element on the
`screen`. You can read more about it on the links below:

- https://stackoverflow.com/questions/69711888/react-testing-library-getbyrole-is-performing-extremely-slowly
- https://github.com/testing-library/dom-testing-library/issues/552#issuecomment-625172052

Even with `ByRole` having performance issues we still want to use it but for
that, we have to scope the "querying" area by using the `within` command. So
instead of using `screen.getByRole("button")` directly we could do
`within(form).getByRole("button")`.

❌ Not ideal. If the screen has a hundred or thousand elements it can be VERY
slow.

```tsx
user.click(screen.getByRole("button"));
```

✅ Better. We can limit the number of elements we are querying.

```tsx
const form = screen.getByTestId("form");
user.click(within(form).getByRole("button"));
```

#### `jest.spyOn` with the API is not working

For some unknown reason, we figured out the `jest.spyOn` is not able to mock the
API function when they are passed directly into the services XState machine
configuration.

❌ Does not work

```ts
import { getUpdateCheck } from "api/api"

createMachine({ ... }, {
  services: {
    getUpdateCheck,
  },
})
```

✅ It works

```ts
import { getUpdateCheck } from "api/api"

createMachine({ ... }, {
  services: {
    getUpdateCheck: () => getUpdateCheck(),
  },
})
```
