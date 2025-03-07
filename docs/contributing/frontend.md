# Frontend

Welcome to the guide for contributing to the Coder frontend. Whether you’re part
of the community or a Coder team member, this documentation will help you get
started.

If you have any questions, feel free to reach out on our
[Discord server](https://discord.com/invite/coder), and we’ll be happy to assist
you.

## Running the UI

You can run the UI and access the Coder dashboard in two ways:

1. Build the UI pointing to an external Coder server:
   `CODER_HOST=https://mycoder.com pnpm dev` inside of the `site` folder. This
   is helpful when you are building something in the UI and already have the
   data on your deployed server.
2. Build the entire Coder server + UI locally: `./scripts/develop.sh` in the
   root folder. This is useful for contributing to features that are not
   deployed yet or that involve both the frontend and backend.

In both cases, you can access the dashboard on `http://localhost:8080`. If using
`./scripts/develop.sh` you can log in with the default credentials.

> [!NOTE]
> **Default Credentials:** `admin@coder.com` and `SomeSecurePassword!`.

## Tech Stack Overview

All our dependencies are described in `site/package.json`, but the following are
the most important.

- [React](https://reactjs.org/) for the UI framework
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
- [PNPM](https://pnpm.io/) as the package manager

## Structure

All UI-related code is in the `site` folder. Key directories include:

- **e2e** - End-to-end (E2E) tests
- **src** - Source code
  - **mocks** - [Manual mocks](https://jestjs.io/docs/manual-mocks) used by Jest
  - **@types** - Custom types for dependencies that don't have defined types
    (largely code that has no server-side equivalent)
  - **api** - API function calls and types
    - **queries** - react-query queries and mutations
  - **components** - Reusable UI components without Coder specific business
    logic
  - **hooks** - Custom React hooks
  - **modules** - Coder-specific UI components
  - **pages** - Page-level components
  - **testHelpers** - Helper functions for integration testing
  - **theme** - theme configuration and color definitions
  - **util** - Helper functions that can be used across the application
- **static** - Static assets like images, fonts, icons, etc

## Routing

We use [react-router](https://reactrouter.com/en/main) as our routing engine.

- Authenticated routes - Place routes requiring authentication inside the
  `<RequireAuth>` route. The `RequireAuth` component handles all the
  authentication logic for the routes.
- Dashboard routes - routes that live in the dashboard should be placed under
  the `<DashboardLayout>` route. The `DashboardLayout` adds a navbar and passes
  down common dashboard data.

## Pages

Page components are the top-level components of the app and reside in the
`src/pages` folder. Each page should have its own folder to group relevant
views, tests, and utility functions. The page component fetches necessary data
and passes to the view. We explain this decision a bit better in the next
section which talks about where to fetch data.

> ℹ️ If code within a page becomes reusable across other parts of the app,
> consider moving it to `src/utils`, `hooks`, `components`, or `modules`.

### Handling States

A page typically has three states: **loading**, **ready**/**success**, and
**error**. Ensure you manage these states when developing pages. Use visual
tests for these states with `*.stories.ts` files.

## Data Fetching

We use [TanStack Query v4](https://tanstack.com/query/v4/docs/react/quick-start)
to fetch data from the API. Queries and mutation should be placed in the
api/queries folder.

### Where to fetch data

In the past, our approach involved creating separate components for page and
view, where the page component served as a container responsible for fetching
data and passing it down to the view.

For instance, when developing a page to display users, we would have a
`UsersPage` component with a corresponding `UsersPageView`. The `UsersPage`
would handle API calls, while the `UsersPageView` managed the presentational
logic.

Over time, however, we encountered challenges with this approach, particularly
in terms of excessive props drilling. To address this, we opted to fetch data in
proximity to its usage. Taking the example of displaying users, in the past, if
we were creating a header component for that page, we would have needed to fetch
the data in the page component and pass it down through the hierarchy
(`UsersPage -> UsersPageView -> UsersHeader`). Now, with libraries such as
`react-query`, data fetching can be performed directly in the `UsersHeader`
component, allowing UI elements to declare and consume their data-fetching
dependencies directly, while preventing duplicate server requests
([more info](https://github.com/TanStack/query/discussions/608#discussioncomment-29735)).

To simplify visual testing of scenarios where components are responsible for
fetching data, you can easily set the queries' value using `parameters.queries`
within the component's story.

```tsx
export const WithQuota: Story = {
    parameters: {
        queries: [
            {
                key: getWorkspaceQuotaQueryKey(MockUser.username),
                data: {
                    credits_consumed: 2,
                    budget: 40,
                },
            },
        ],
    },
};
```

### API

Our project uses [axios](https://github.com/axios/axios) as the HTTP client for
making API requests. The API functions are centralized in `site/src/api/api.ts`.
Auto-generated TypeScript types derived from our Go server are located in
`site/src/api/typesGenerated.ts`.

Typically, each API endpoint corresponds to its own `Request` and `Response`
types. However, some endpoints require additional parameters for successful
execution. Here's an illustrative example:"

```ts
export const getAgentListeningPorts = async (
    agentID: string,
): Promise<TypesGen.ListeningPortsResponse> => {
    const response = await axiosInstance.get(
        `/api/v2/workspaceagents/${agentID}/listening-ports`,
    );
    return response.data;
};
```

Sometimes, a frontend operation can have multiple API calls which can be wrapped
as a single function.

```ts
export const updateWorkspaceVersion = async (
    workspace: TypesGen.Workspace,
): Promise<TypesGen.WorkspaceBuild> => {
    const template = await getTemplate(workspace.template_id);
    return startWorkspace(workspace.id, template.active_version_id);
};
```

## Components and Modules

Components should be atomic, reusable and free of business logic. Modules are
similar to components except that they can be more complex and can contain
business logic specific to the product.

### MUI

The codebase is currently using MUI v5. Please see the
[official documentation](https://mui.com/material-ui/getting-started/). In
general, favor building a custom component via MUI instead of plain React/HTML,
as MUI's suite of components is thoroughly battle-tested and accessible right
out of the box.

### Structure

Each component and module gets its own folder. Module folders may group multiple
files in a hierarchical structure. Storybook stories and component tests using
Storybook interactions are required. By keeping these tidy, the codebase will
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

### Should I create a new component or module?

Components could technically be used in any codebase and still feel at home. A
module would only make sense in the Coder codebase.

- Component
  - Simple
  - Atomic, used in multiple places
  - Generic, would be useful as a component outside of the Coder product
  - Good Examples: `Badge`, `Form`, `Timeline`
- Module
  - Simple or Complex
  - Used in multiple places
  - Good Examples: `Provisioner`, `DashboardLayout`, `DeploymentBanner`

Our codebase has some legacy components that are being updated to follow these
new conventions, but all new components should follow these guidelines.

## Styling

We use [Emotion](https://emotion.sh/) to handle css styles.

## Forms

We use [Formik](https://formik.org/docs) for forms along with
[Yup](https://github.com/jquense/yup) for schema definition and validation.

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

We use visual tests to test components without user interaction like testing if
a page/component is rendered correctly depending on some parameters, if a button
is showing a spinner, if `loading` props are passed correctly, etc. This should
always be your first option since it is way easier to maintain. For this, we use
[Storybook](https://storybook.js.org/) and
[Chromatic](https://www.chromatic.com/).

> ℹ️ To learn more about testing components that fetch API data, refer to the
> [**Where to fetch data**](#where-to-fetch-data) section.

### What should I test?

Choosing what to test is not always easy since there are a lot of flows and a
lot of things can happen but these are a few indicators that can help you with
that:

- Things that can block the user
- Reported bugs
- Regression issues

### Tests getting too slow

You may have observed that certain tests in our suite can be notably
time-consuming. Sometimes it is because the test itself is complex and sometimes
it is because of how the test is querying elements.

#### Using `ByRole` queries

One thing we figured out that was slowing down our tests was the use of `ByRole`
queries because of how it calculates the role attribute for every element on the
`screen`. You can read more about it on the links below:

- <https://stackoverflow.com/questions/69711888/react-testing-library-getbyrole-is-performing-extremely-slowly>
- <https://github.com/testing-library/dom-testing-library/issues/552#issuecomment-625172052>

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
