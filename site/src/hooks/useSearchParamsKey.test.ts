import { useSearchParamsKey } from "./useSearchParamsKey";
import { renderHookWithAuth } from "testHelpers/renderHelpers";
import { act, waitFor } from "@testing-library/react";

/**
 * Tried to extract the setup logic into one place, but it got surprisingly
 * messy. Went with straightforward approach of calling things individually
 *
 * @todo See if there's a way to test the interaction with the history object
 * (particularly, for replace behavior). It's traditionally very locked off, and
 * React Router gives you no way of interacting with it directly.
 */
describe(useSearchParamsKey.name, () => {
  it("Returns out a default value of an empty string if the key does not exist in URL", async () => {
    const { result } = await renderHookWithAuth(
      () => useSearchParamsKey("blah"),
      { routingOptions: { route: `/` } },
    );

    expect(result.current.value).toEqual("");
  });

  it("Uses the 'defaultValue' config override if provided", async () => {
    const defaultValue = "dogs";
    const { result } = await renderHookWithAuth(
      () => useSearchParamsKey("blah", { defaultValue }),
      { routingOptions: { route: `/` } },
    );

    expect(result.current.value).toEqual(defaultValue);
  });

  it("Is able to read to read keys from the URL on mounting render", async () => {
    const key = "blah";
    const value = "cats";

    const { result } = await renderHookWithAuth(() => useSearchParamsKey(key), {
      routingOptions: {
        route: `/?${key}=${value}`,
      },
    });

    expect(result.current.value).toEqual(value);
  });

  it("Updates state and URL when the setValue callback is called with a new value", async () => {
    const key = "blah";
    const initialValue = "cats";

    const { result, getLocationSnapshot } = await renderHookWithAuth(
      () => useSearchParamsKey(key),
      {
        routingOptions: {
          route: `/?${key}=${initialValue}`,
        },
      },
    );

    const newValue = "dogs";
    act(() => result.current.onValueChange(newValue));
    await waitFor(() => expect(result.current.value).toEqual(newValue));

    const { search } = getLocationSnapshot();
    expect(search.get(key)).toEqual(newValue);
  });

  it("Clears value for the given key from the state and URL when removeValue is called", async () => {
    const key = "blah";
    const initialValue = "cats";

    const { result, getLocationSnapshot } = await renderHookWithAuth(
      () => useSearchParamsKey(key),
      {
        routingOptions: {
          route: `/?${key}=${initialValue}`,
        },
      },
    );

    act(() => result.current.removeValue());
    await waitFor(() => expect(result.current.value).toEqual(""));

    const { search } = getLocationSnapshot();
    expect(search.get(key)).toEqual(null);
  });

  it("Does not have methods change previous values if 'key' argument changes during re-renders", async () => {
    const readonlyKey = "readonlyKey";
    const mutableKey = "mutableKey";
    const initialReadonlyValue = "readonly";
    const initialMutableValue = "mutable";

    const { result, rerender, getLocationSnapshot } = await renderHookWithAuth(
      ({ key }) => useSearchParamsKey(key),
      {
        routingOptions: {
          route: `/?${readonlyKey}=${initialReadonlyValue}&${mutableKey}=${initialMutableValue}`,
        },

        renderOptions: {
          initialProps: { key: readonlyKey },
        },
      },
    );

    const swapValue = "dogs";
    rerender({ key: mutableKey });
    act(() => result.current.onValueChange(swapValue));
    await waitFor(() => expect(result.current.value).toEqual(swapValue));

    const { search } = getLocationSnapshot();
    expect(search.get(readonlyKey)).toEqual(initialReadonlyValue);
    expect(search.get(mutableKey)).toEqual(swapValue);
  });
});
