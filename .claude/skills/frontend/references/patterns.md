# Frontend Pattern Examples

Correct/incorrect code examples for the FE1 to FE10 rules. The rule contract
itself lives in the skill body (`../SKILL.md`); this file holds the examples so
they load only when needed.

## FE1: Storybook interaction coverage

Incorrect (interaction test in Vitest/RTL):

```tsx
// ModelSelector.test.tsx
it("selects a model", async () => {
  render(<ModelSelector {...props} />);
  await userEvent.click(screen.getByRole("button"));
  // ...
});
```

Correct (Storybook story with a play function):

```tsx
// ModelSelector.stories.tsx
export const SelectModel: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: /model/i }));
    await expect(canvas.getByRole("listbox")).toBeVisible();
  },
};
```

## FE2: No loose types

Incorrect:

```tsx
const config = data as unknown as ChatModel;
```

Correct:

```tsx
import type { ChatModel } from "api/typesGenerated";

const config: ChatModel = parseConfig(data);
```

## FE3: Reuse before building

Incorrect (hand-assembling a primitive that already exists as a wrapped
component, or re-creating a near-duplicate helper):

```tsx
// A new StatusPill that duplicates the shared Badge
const StatusPill = ({ label }: { label: string }) => (
  <span className="rounded-full border px-2 py-0.5 text-xs">{label}</span>
);
```

Correct (reuse the shared primitive):

```tsx
import { Badge } from "components/Badge/Badge";

<Badge size="sm">{label}</Badge>;
```

## FE4: Comments must earn their place

Incorrect:

```tsx
// Track whether the panel is open
const [isOpen, setIsOpen] = useState(false);
```

## FE5: UI state matrix

| State   | Requirement                                                  |
|---------|--------------------------------------------------------------|
| Loading | Show a skeleton or spinner, never a blank or half-valid view |
| Error   | Surface the actionable server error, not a generic message   |
| Empty   | Deliberate empty state with copy, never a blank region       |
| Refetch | Keep showing valid data; never reset forms or selections     |

## FE7: React Query discipline

Incorrect (re-typed query key in a story):

```tsx
parameters: {
  queries: [{ key: ["chat-models"], data: [MockChatModel] }],
},
```

Correct (imported constant):

```tsx
import { chatModelsKey } from "api/queries/chats";

parameters: {
  queries: [{ key: chatModelsKey, data: [MockChatModel] }],
},
```

## FE6: Accessibility is behavior

Incorrect (an `aria-label` that replaces the visible label, a "Label in Name"
violation):

```tsx
<Button aria-label="submit">Create workspace</Button>
```

Correct (accessible name contains the visible text):

```tsx
<Button>Create workspace</Button>
```

## FE8: Effects are a last resort

Incorrect (deriving state in an effect, and memoizing a callback preemptively):

```tsx
const [fullName, setFullName] = useState("");
useEffect(() => {
  setFullName(`${first} ${last}`);
}, [first, last]);

const onSave = useCallback(() => save(fullName), [fullName]);
```

Correct (derive during render; no `useCallback` until a memoized consumer needs
a stable reference):

```tsx
const fullName = `${first} ${last}`;

const onSave = () => save(fullName);
```

## FE9: Fixtures and mocks follow repo conventions

Incorrect (inline entity literal that duplicates a shared fixture):

```tsx
const config = { id: "1", name: "gpt", provider: "openai" /* ... */ };
```

Correct (spread the shared `Mock*` fixture for a variant):

```tsx
import { MockChatModel } from "testHelpers/chatModels";

const config = { ...MockChatModel, name: "gpt" };
```

## FE10: Tests assert observable behavior

Incorrect (class-name substring lookup):

```tsx
const row = canvasElement.querySelector("[class*='flex-col-reverse']");
```

Correct (query by role and accessible name):

```tsx
const row = canvas.getByRole("row", { name: /workspace/i });
```
