# Chat sharing mobile design

## Problem statement

The Coder Agents chat sharing popup uses a fixed-width desktop popover. On mobile viewports the content can overflow horizontally, and the add-member controls plus member table do not fit comfortably. The interaction should remain recognizable as the current chat sharing UI while becoming usable on small screens.

## Approved design

Use a responsive version of the existing popover rather than introducing a separate mobile dialog.

Desktop behavior remains visually equivalent to the current design:

- The top bar share trigger opens a popover aligned to the trigger.
- The popover uses the existing surface, border, radius, shadow, spacing, title, alerts, loading state, autocomplete, buttons, table, avatar rows, role badge, and row menu patterns.
- The desktop member list remains table-based with the existing columns for member, role, and actions.

Mobile behavior changes the layout inside the same design language:

- The popover content becomes viewport-safe instead of fixed at 580px.
- Mobile padding and spacing are reduced enough to preserve content density without changing tokens or visual style.
- The add-member controls stack vertically. The autocomplete uses the available width, and the add button sits below it with an appropriate touch-friendly width.
- The member list uses compact stacked rows on small screens rather than the desktop table columns.
- Each mobile row keeps the same information and controls as desktop: avatar, title, subtitle, read role badge, and remove menu.
- Empty, loading, ACL error, and mutation error states remain unchanged in meaning and should fit inside the mobile popover.

## Rejected alternatives

### Minimal width-only popover

This would reduce implementation risk, but the table and inline add controls would still be cramped on mobile. It addresses overflow but not the underlying mobile usability problem.

### Mobile dialog or sheet

A dialog could handle small screens well, but it changes the interaction model more than necessary and adds risk with nested autocomplete and menu popovers. The requested goal is to keep the current design language, so improving the existing popover is a better first step.

## Edge cases

- Long user names, group names, or subtitles should truncate or wrap without forcing horizontal page overflow.
- Empty ACL should remain understandable on mobile.
- Loading and error states should stay visible without requiring horizontal scrolling.
- The current user must still be hidden from the shared member list and excluded from autocomplete.
- Mutation errors should continue to clear across member types and when the popover is reopened.
- Remove menus should remain reachable by touch and keyboard.
- Nested autocomplete popovers should not become clipped by the responsive parent layout.

## Verification plan

- Add or update Storybook stories for mobile chat sharing at a 390px viewport.
- Verify populated ACL mobile story displays user, group, read roles, and row menus without horizontal overflow.
- Run the focused Storybook test for `ChatSharingPopover.stories.tsx`.
- Run frontend formatting for touched files.
- Run the focused frontend lint or typecheck path required by the repository if the focused test exposes TypeScript issues.

## Open questions

None.
