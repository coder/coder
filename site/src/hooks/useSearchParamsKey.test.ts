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
  it("Returns out an empty string as the default value if the hook's key does not exist in URL", async () => {
    const { result } = await renderHookWithAuth(
      () => useSearchParamsKey({ key: "blah" }),
      { routingOptions: { route: `/` } },
    );

    expect(result.current.value).toEqual("");
  });

  it("Uses the 'defaultValue' config override if provided", async () => {
    const defaultValue = "dogs";
    const { result } = await renderHookWithAuth(
      () => useSearchParamsKey({ key: "blah", defaultValue }),
      { routingOptions: { route: `/` } },
    );

    expect(result.current.value).toEqual(defaultValue);
  });

  it("Uses the URL key if it exists, regardless of render, always ignoring the default value", async () => {
    const key = "blah";
    const value = "cats";

    const { result } = await renderHookWithAuth(
      () => useSearchParamsKey({ key, defaultValue: "I don't matter" }),
      { routingOptions: { route: `/?${key}=${value}` } },
    );

    expect(result.current.value).toEqual(value);
  });

  it("Updates state and URL when the setValue callback is called with a new value", async () => {
    const key = "blah";
    const initialValue = "cats";

    const { result, getLocationSnapshot } = await renderHookWithAuth(
      () => useSearchParamsKey({ key }),
      { routingOptions: { route: `/?${key}=${initialValue}` } },
    );

    const newValue = "dogs";
    void act(() => result.current.setValue(newValue));
    await waitFor(() => expect(result.current.value).toEqual(newValue));

    const { search } = getLocationSnapshot();
    expect(search.get(key)).toEqual(newValue);
  });

  it("Clears value for the given key from the state and URL when removeValue is called", async () => {
    const key = "blah";
    const initialValue = "cats";

    const { result, getLocationSnapshot } = await renderHookWithAuth(
      () => useSearchParamsKey({ key }),
      { routingOptions: { route: `/?${key}=${initialValue}` } },
    );

    void act(() => result.current.deleteValue());
    await waitFor(() => expect(result.current.value).toEqual(""));

    const { search } = getLocationSnapshot();
    expect(search.get(key)).toEqual(null);
  });

  it("Will dispatch state changes through custom URLSearchParams value if provided", async () => {
    const key = "love";
    const initialValue = "dogs";
    const customParams = new URLSearchParams({ [key]: initialValue });

    const { result } = await renderHookWithAuth(
      ({ key }) => useSearchParamsKey({ key, searchParams: customParams }),
      {
        routingOptions: { route: `/?=${key}=${initialValue}` },
        renderOptions: { initialProps: { key } },
      },
    );

    const newValue = "all animals";
    void act(() => result.current.setValue(newValue));
    await waitFor(() => expect(customParams.get(key)).toEqual(newValue));
  });

  it("Does not have methods change previous values if 'key' argument changes during re-renders", async () => {
    const readonlyKey = "readonlyKey";
    const mutableKey = "mutableKey";
    const initialReadonlyValue = "readonly";
    const initialMutableValue = "mutable";

    const { result, rerender, getLocationSnapshot } = await renderHookWithAuth(
      ({ key }) => useSearchParamsKey({ key }),
      {
        routingOptions: {
          route: `/?${readonlyKey}=${initialReadonlyValue}&${mutableKey}=${initialMutableValue}`,
        },
        renderOptions: { initialProps: { key: readonlyKey } },
      },
    );

    const swapValue = "dogs";
    rerender({ key: mutableKey });
    void act(() => result.current.setValue(swapValue));
    await waitFor(() => expect(result.current.value).toEqual(swapValue));

    const { search } = getLocationSnapshot();
    expect(search.get(readonlyKey)).toEqual(initialReadonlyValue);
    expect(search.get(mutableKey)).toEqual(swapValue);
  });
});
