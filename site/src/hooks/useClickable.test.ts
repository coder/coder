import { useClickable } from "./useClickable";

/**
 * Gut feeling is that I might want a dummy element to test more user-like
 * behavior against this
 *
 * Things to test:
 * 1. Passing a custom role into the function will give you the same role in
 *    the return value
 * 2. Function defaults to role "button"
 * 3. Firing keyboard events will cause the element to simulate click behavior,
 *    but only if the element is focused (test both Enter and Space)
 * 4. Element is focusable
 * 5. Element's semantics change to reflect a more semantic button-like element
 * 6. Holding down Enter will continually fire events
 * 7. Holding down Space will NOT continually fire events
 * 8. If focus is lost from the button while space was held down, releasing the
 *    key will NOT cause anything to fire
 */
describe(useClickable.name, () => {
  it.skip("", () => {
    expect.hasAssertions();
  });
});
