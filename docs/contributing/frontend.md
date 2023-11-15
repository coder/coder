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
- [axios](https://github.com/axios/axios) as fetching lib
- [Playwright](https://playwright.dev/) for end-to-end (E2E) testing
- [Jest](https://jestjs.io/) for integration testing
- [Storybook](https://storybook.js.org/) and
  [Chromatic](https://www.chromatic.com/) for visual testing
- [PNPM](https://pnpm.io/) as package manager

## Structure

All the code related to the UI is inside the `site` folder and we defined a few
conventions to help people to navigate through it.

- **e2e** - End-to-end (E2E) tests
- **src** - Source code
  - **mocks** - [Manual mocks](https://jestjs.io/docs/manual-mocks) used by Jest
  - **@types** - Custom types for dependencies that don't have defined types
    (largely code that has no server-side equivalent)
  - **api** - API code as function calls and types
  - **components** - UI components
  - **hooks** - Hooks that can be used across the application
  - **pages** - Page components
  - **testHelpers** - Helper functions to help with integration tests
  - **util** - Helper functions that can be used across the application
  - **xServices** - XState machines used to handle complex state representations
- **static** - Static UI assets like images, fonts, icons, etc

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

A page usually has at least three states: **loading**, **ready**/**success**,
and **error**, so always remember to handle these scenarios while you are coding
a page. We also encourage you to add visual testing for these three states using
a `*.stories.ts` file.

## Fetching data

We use
[TanStack Query v4](https://tanstack.com/query/v4/docs/react/overview)(previously
known as react-query) to fetch data from the API.

### Where to fetch data

Finding the right place to fetch data in React apps is the million-dollar
question, but we decided to make it only in the page components and pass the
props down to the views. This makes it easier to find where data is being loaded
and easy to test using Storybook. So you will see components like `UsersPage`
and `UsersPageView`.

### API

We are using [axios](https://github.com/axios/axios) as our fetching library and
writing the API functions in the `site/src/api/api.ts` files. We also have
auto-generated types from our Go server on `site/src/api/typesGenerated.ts`.
Usually, every endpoint has its own ` Request` and `Response` types, but
sometimes you need to pass extra parameters to make the call, like in the
example below:

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

Sometimes, a frontend operation can have multiple API calls, so it is okay to
wrap it as a single function.

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

The codebase is currently using MUI v5. Please see the
[official documentation](https://mui.com/material-ui/getting-started/). In
general, favor building a custom component via MUI instead of plain React/HTML,
as MUI's suite of components is thoroughly battle-tested and accessible right
out of the box.

### Structure

Each component gets its own folder. Make sure you add a test and Storybook
stories for the component as well. By keeping these tidy, the codebase will
remain easy to navigate, healthy and maintainable for all contributors.

### Accessibility

We strive to keep our UI accessible.

In general, colors should come from the app theme, but if there is a need to add
a custom color, please ensure that the foreground and background have a minimum
contrast ratio of 4.5:1 to meet WCAG level AA compliance. WebAIM has
[a great tool for checking your colors directly](https://webaim.org/resources/contrastchecker/),
but tools like
[Dequeue's axe DevTools](https://chrome.google.com/webstore/detail/axe-devtools-web-accessib/lhdoppojpmngadmnindnejefpokejbdd)
can also do automated checks in certain situations.

When using any kind of input element, always make sure that there is a label
associated with that element (the label can be made invisible for aesthetic
reasons, but it should always be in the HTML markup). Labels are important for
screen-readers; a placeholder text value is not enough for all users.

When possible, make sure that all image/graphic elements have accompanying text
that describes the image. `<img />` elements should have an `alt` text value. In
other situations, it might make sense to place invisible, descriptive text
inside the component itself using MUI's `visuallyHidden` utility function.

```tsx
import { visuallyHidden } from "@mui/utils";

<Button>
  <GearIcon />
  <Box component="span" sx={visuallyHidden}>
    Settings
  </Box>
</Button>;
```

### Should I create a new component?

As with most things in the world, it depends. If you are creating a new
component to encapsulate some UI abstraction like `UsersTable` it is ok but you
should always try to use the base components that are provided by the library or
from the codebase. It's recommended that you always do a quick search before
creating a custom primitive component like dialogs, popovers, buttons, etc.

## Testing

We use three types of testing in our app: **End-to-end (E2E)**, **Integration**
and **Visual Testing**.

### End-to-End (E2E)

These are useful for testing complete flows like "Create a user", "Import
template", etc. We use [Playwright](https://playwright.dev/). If you only need
to test if the page is being rendered correctly, you should consider using the
**Visual Testing** approach.

> ℹ️ For scenarios where you need to be authenticated, you can use
> `test.use({ storageState: getStatePath("authState") })`.

For ease of debugging, it's possible to run a Playwright test in headful mode
running a Playwright server on your local machine, and executing the test inside
your workspace.

You can either run `scripts/remote_playwright.sh` from `coder/coder` on your
local machine, or execute the following command if you don't have the repo
available:

```bash
bash <(curl -sSL https://raw.githubusercontent.com/coder/coder/main/scripts/remote_playwright.sh) [workspace]
```

The `scripts/remote_playwright.sh` script will start a Playwright server on your
local machine and forward the necessary ports to your workspace. At the end of
the script, you will land _inside_ your workspace with environment variables set
so you can simply execute the test (`pnpm run playwright:test`).

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
