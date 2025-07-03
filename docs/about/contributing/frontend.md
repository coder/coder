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

## Development Commands

All commands should be run from the `site/` directory:

```bash
# Development
pnpm dev                    # Start Vite development server
pnpm storybook --no-open    # Run Storybook for component development

# Testing
pnpm test                   # Run Jest unit tests
pnpm test -- path/to/file   # Run specific test file
pnpm playwright:test        # Run Playwright E2E tests (requires license)

# Code Quality
pnpm lint                   # Run complete linting suite (Biome + TypeScript + circular deps + knip)
pnpm lint:fix               # Auto-fix linting issues where possible
pnpm format                 # Format code with Biome (always run before PR)
pnpm check                  # Type-check with TypeScript

# Build
pnpm build                  # Production build
```

### Pre-PR Checklist

Before creating a pull request, ensure you run:

1. `pnpm check` - Ensure no TypeScript errors
2. `pnpm lint` - Fix linting issues
3. `pnpm format` - Format code consistently
4. `pnpm test` - Run affected unit tests
5. Check Storybook for any components affected by code changes

## Tech Stack Overview

All our dependencies are described in `site/package.json`, but the following are
the most important.

- [React](https://reactjs.org/) for the UI framework
- [Typescript](https://www.typescriptlang.org/) to keep our sanity
- [Vite](https://vitejs.dev/) to build the project
- [TailwindCSS](https://tailwindcss.com/) for styling (migrating from Emotion)
- [shadcn/ui](https://ui.shadcn.com/) + [Radix UI](https://www.radix-ui.com/) for UI components (migrating from Material UI)
- [Lucide React](https://lucide.dev/) for icons
- [react-router](https://reactrouter.com/en/main) for routing
- [TanStack Query v4](https://tanstack.com/query/v4/docs/react/overview) for
  fetching data
- [axios](https://github.com/axios/axios) as fetching lib
- [Playwright](https://playwright.dev/) for end-to-end (E2E) testing
- [Jest](https://jestjs.io/) for integration testing
- [Storybook](https://storybook.js.org/) and
  [Chromatic](https://www.chromatic.com/) for visual testing
- [Biome](https://biomejs.dev/) for linting and formatting
- [PNPM](https://pnpm.io/) as the package manager

## Migration Status

**⚠️ Important: We are currently migrating from Material UI (MUI) to shadcn/ui and from Emotion to TailwindCSS.**

### Current State

- **~210 files** still use MUI components (`@mui/material`)
- **~41 components** have been migrated to use TailwindCSS classes
- **shadcn/ui components** are being added incrementally to `src/components/`
- **Emotion CSS** is deprecated but still present in legacy components

### Migration Guidelines

When working on existing components:

These instructions assume that you have already checked the `src/components` directory, and it doesn't have the right component you need.
1. **Prefer components in this order:** `shadcn/ui`, Radix, MUI
2. **Use TailwindCSS classes** instead of Emotion's `css` prop or MUI's `sx` prop
3. **Check `src/components/`** for existing shadcn/ui implementations before creating new ones
4. **Do not use the shadcn CLI** - manually add components to maintain consistency
5. **Update tests** to reflect new component structure when migrating

For new components:

1. **Always use TailwindCSS** for styling
2. **Use shadcn/ui components** as the foundation
3. **Use Lucide React icons** instead of MUI icons
4. **Follow the semantic color tokens** defined in `tailwind.config.js`

### Semantic Color System

Use the custom semantic color tokens defined in our Tailwind configuration:

- **Text/foreground content colors**
  - `primary`
  - `secondary`
  - `disabled`
  - `invert`
  - `success`
  - `link`
  - `destructive`
  - `warning`
- **Surface colors**
  - `primary`
  - `secondary`
  - `tertiary`
  - `quaternary`
  - `invert-primary`
  - `invert-secondary`
  - `destructive`
  - `green`
  - `grey`
  - `orange`
  - `sky`
  - `red`
  - `purple`
- **Border colors**
  - `default`
  - `warning`
  - `destructive`
  - `success`
  - `hover`
- **Highlight colors**
  - `purple`
  - `green`
  - `grey`
  - `sky`
  - `red`

Variants are configured for each of Tailwind's color-based classes (`text`, `bg`, `fill`, etc.). For example, to set the text color for primary content, use the `text-content-primary` class. To make a `<div>` use warning colors, use `bg-warning` and `border-warning`.

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

If code within a page becomes reusable across other parts of the app,
consider moving it to `src/utils`, `hooks`, `components`, or `modules`.

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
                key: getWorkspaceQuotaQueryKey(MockUserOwner.username),
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

### UI Components

**⚠️ MUI is deprecated** - we are migrating to shadcn/ui + Radix UI.

#### shadcn/ui Components (Preferred)

We use [shadcn/ui](https://ui.shadcn.com/) components built on top of [Radix UI](https://www.radix-ui.com/) primitives. These components are:

- **Accessible by default** with ARIA attributes and keyboard navigation
- **Customizable** with TailwindCSS classes
- **Consistent** with our design system
- **Type-safe** with full TypeScript support

Existing shadcn/ui components can be found in `src/components/`. Examples include:

- `Checkbox` - Form checkbox input
- `ScrollArea` - Custom scrollable area
- `Table` - Data table with sorting and filtering
- `Slider` - Range input slider
- `Switch` - Toggle switch
- `Command` - Command palette/search
- `Collapsible` - Expandable content sections

#### MUI Components (Legacy)

The codebase still contains MUI v5 components that are being phased out. When encountering MUI components:

1. **Check if a shadcn/ui equivalent exists** in `src/components/`
2. **Migrate to the shadcn/ui version** when making changes
3. **Create a new shadcn/ui component** if no equivalent exists
4. **Do not add new MUI components** to the codebase

For reference, the [MUI documentation](https://mui.com/material-ui/getting-started/) can still be consulted for understanding existing legacy components.

### Structure

Each component and module gets its own folder. Module folders may group multiple
files in a hierarchical structure. Storybook stories and component tests using
Storybook interactions are required. By keeping these tidy, the codebase will
remain easy to navigate, healthy and maintainable for all contributors.

### Accessibility

We strive to keep our UI accessible. **shadcn/ui components are accessible by default** with proper ARIA attributes, keyboard navigation, and focus management.

#### Color Contrast

Colors should come from our semantic color tokens in the Tailwind theme. These tokens are designed to meet WCAG level AA compliance (4.5:1 contrast ratio). If you need to add a custom color, ensure proper contrast using:

- [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/)
- [Dequeue's axe DevTools](https://chrome.google.com/webstore/detail/axe-devtools-web-accessib/lhdoppojpmngadmnindnejefpokejbdd)

#### Form Labels

Always associate labels with input elements. Labels can be visually hidden but must be present in the markup for screen readers.

```tsx
// ✅ Good: Visible label
<label htmlFor="email" className="text-content-primary">
  Email Address
</label>
<input id="email" type="email" className="border border-default" />

// ✅ Good: Visually hidden label
<label htmlFor="search" className="sr-only">
  Search
</label>
<input id="search" placeholder="Search..." className="border border-default" />
```

#### Images and Icons

Provide descriptive text for images and icons:

```tsx
// ✅ Good: Alt text for images
<img src="chart.png" alt="Revenue increased 25% this quarter" />

// ✅ Good: Screen reader text for icons
<button className="p-2">
  <GearIcon aria-hidden="true" />
  <span className="sr-only">Settings</span>
</button>

// ✅ Good: Using Lucide React icons with proper labeling
import { Settings } from "lucide-react";
<button>
  <Settings className="w-4 h-4" aria-hidden="true" />
  <span className="sr-only">Open settings</span>
</button>
```

#### Legacy MUI Accessibility

For legacy MUI components, you may still see the `visuallyHidden` utility:

```tsx
// ❌ Legacy: MUI visuallyHidden
import { visuallyHidden } from "@mui/utils";
<Box component="span" sx={visuallyHidden}>Settings</Box>

// ✅ Migrated: Tailwind sr-only class
<span className="sr-only">Settings</span>
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

**⚠️ Emotion is deprecated** - we are migrating to TailwindCSS.

### TailwindCSS (Preferred)

We use [TailwindCSS](https://tailwindcss.com/) for all new styling. Key points:

- **Use semantic color tokens** from our custom theme (see Migration Status section above)
- **Responsive design** with Tailwind's responsive prefixes (`sm:`, `md:`, `lg:`, `xl:`)
- **No dark mode prefix** - our theme handles light/dark mode automatically
- **Custom utilities** are defined in `tailwind.config.js`
- **Conditional styling** using `clsx` utility for dynamic classes

#### TailwindCSS Best Practices

```tsx
// ✅ Good: Use semantic color tokens
<div className="bg-surface-primary text-content-primary border border-default">

// ✅ Good: Group related classes
<div className="flex items-center justify-between p-4 rounded-lg">

// ✅ Good: Responsive design
<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3">

// ✅ Good: Conditional styling with clsx
import { clsx } from "clsx";
<button className={clsx(
  "px-4 py-2 rounded",
  isActive && "bg-surface-tertiary",
  isDisabled && "opacity-50 cursor-not-allowed"
)}>
```

### Emotion (Legacy)

Legacy components may still use [Emotion](https://emotion.sh/) with the `css` prop or `sx` prop from MUI. When working with these:

1. **Migrate to TailwindCSS classes** when making changes
2. **Remove Emotion imports** after migration
3. **Update tests** to reflect new class-based styling

```tsx
// ❌ Legacy: Emotion css prop
<div css={{ padding: 16, backgroundColor: 'white' }}>

// ✅ Migrated: TailwindCSS classes
<div className="p-4 bg-surface-primary">
```

## Forms

We use [Formik](https://formik.org/docs) for forms along with
[Yup](https://github.com/jquense/yup) for schema definition and validation.

## Testing

We use three types of testing in our app: **End-to-end (E2E)**, **Integration/Unit**
and **Visual Testing**.

### End-to-End (E2E) – Playwright

These are useful for testing complete flows like "Create a user", "Import
template", etc. We use [Playwright](https://playwright.dev/). These tests run against a full Coder instance, backed by a database, and allows you to make sure that features work properly all the way through the stack. "End to end", so to speak.

For scenarios where you need to be authenticated as a certain user, you can use
`login` helper. Passing it some user credentials will log out of any other user account, and will attempt to login using those credentials.

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

### Integration/Unit – Jest

We use Jest mostly for testing code that does _not_ pertain to React. Functions and classes that contain notable app logic, and which are well abstracted from React should have accompanying tests. If the logic is tightly coupled to a React component, a Storybook test or an E2E test may be a better option depending on the scenario.

### Visual Testing – Storybook

We use Storybook for testing all of our React code. For static components, you simply add a story that renders the components with the props that you would like to test, and Storybook will record snapshots of it to ensure that it isn't changed unintentionally. If you would like to test an interaction with the component, then you can add an interaction test by specifying a `play` function for the story. For stories with an interaction test, a snapshot will be recorded of the end state of the component. We use
[Chromatic](https://www.chromatic.com/) to manage and compare snapshots in CI.

To learn more about testing components that fetch API data, refer to the
[**Where to fetch data**](#where-to-fetch-data) section.

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

## Migration Guide: MUI to shadcn/ui

This section provides practical guidance for migrating components from MUI to shadcn/ui.

### Step-by-Step Migration Process

1. **Identify the component** you're working on
2. **Check for existing shadcn/ui equivalent** in `src/components/`
3. **Plan the migration** - identify props, styling, and functionality
4. **Update imports** - replace MUI imports with shadcn/ui
5. **Replace styling** - convert Emotion/sx props to TailwindCSS classes
6. **Update tests** - ensure component tests still pass
7. **Test accessibility** - verify keyboard navigation and screen reader support

### Common Migration Patterns

#### Button Migration

```tsx
// ❌ Before: MUI Button
import { Button } from "@mui/material";

<Button 
  variant="contained" 
  color="primary"
  sx={{ margin: 2 }}
  onClick={handleClick}
>
  Click me
</Button>

// ✅ After: shadcn/ui Button (if available) or custom button
import { Button } from "components/Button";

<Button 
  variant="primary"
  className="m-2"
  onClick={handleClick}
>
  Click me
</Button>
```

#### Form Field Migration

```tsx
// ❌ Before: MUI TextField
import { TextField } from "@mui/material";

<TextField
  label="Email"
  variant="outlined"
  error={!!errors.email}
  helperText={errors.email}
  sx={{ marginBottom: 2 }}
/>

// ✅ After: shadcn/ui Input with Label
import { Input } from "components/Input";
import { Label } from "components/Label";

<div className="mb-2">
  <Label htmlFor="email">Email</Label>
  <Input 
    id="email"
    className={errors.email ? "border-border-destructive" : ""}
  />
  {errors.email && (
    <p className="text-content-destructive text-sm mt-1">
      {errors.email}
    </p>
  )}
</div>
```

#### Icon Migration

```tsx
// ❌ Before: MUI Icons
import { Settings as SettingsIcon } from "@mui/icons-material";

<SettingsIcon fontSize="small" />

// ✅ After: Lucide React Icons
import { Settings } from "lucide-react";

<Settings className="w-4 h-4" />
```

### Migration Checklist

When migrating a component, ensure you:

- [ ] Replace MUI imports with shadcn/ui equivalents
- [ ] Convert `sx` props and Emotion `css` to TailwindCSS classes
- [ ] Use semantic color tokens from the theme
- [ ] Update icon imports to use Lucide React
- [ ] Maintain or improve accessibility
- [ ] Update component tests
- [ ] Update Storybook stories if they exist
- [ ] Verify responsive behavior
- [ ] Test keyboard navigation
- [ ] Check color contrast compliance

### Getting Help

If you encounter challenges during migration:

1. **Check existing implementations** in `src/components/` for patterns
2. **Refer to shadcn/ui documentation** at [ui.shadcn.com](https://ui.shadcn.com/)
3. **Ask in Discord** - the team is happy to help with migration questions
4. **Look at recent PRs** for migration examples

### Creating New shadcn/ui Components

When a shadcn/ui equivalent doesn't exist:

1. **Check the shadcn/ui registry** for the component
2. **Copy the component code** (don't use the CLI)
3. **Adapt to our theme** using semantic color tokens
4. **Add to `src/components/`** with proper folder structure
5. **Create Storybook stories** for documentation
6. **Add TypeScript types** for props
7. **Include accessibility features** (ARIA attributes, keyboard support)

Example component structure:

```text
src/components/NewComponent/
├── NewComponent.tsx
├── NewComponent.stories.tsx
├── NewComponent.test.tsx
└── index.ts
```
