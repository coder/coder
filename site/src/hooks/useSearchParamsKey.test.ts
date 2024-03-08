import { act, waitFor } from "@testing-library/react";
import { renderHookWithAuth } from "testHelpers/hooks";
import { useSearchParamsKey } from "./useSearchParamsKey";

/**
 * Tried to extract the setup logic into one place, but it got surprisingly
 * messy. Went with straightforward approach of calling things individually
 *
 * @todo See if there's a way to test the interaction with the history object
 * (particularly, for replace behavior). It's traditionally very locked off, and
 * React Router gives you no way of interacting with it directly.
 */
describe(useSearchParamsKey.name, () => {
  describe("Render behavior", () => {
    it("Returns empty string if hook key does not exist in URL, and there is no default value", async () => {
      const { result } = await renderHookWithAuth(
        () => useSearchParamsKey({ key: "blah" }),
        { routingOptions: { route: `/` } },
      );

      expect(result.current.value).toEqual("");
    });

    it("Returns out 'defaultValue' property if defined while hook key does not exist in URL", async () => {
      const defaultValue = "dogs";
      const { result } = await renderHookWithAuth(
        () => useSearchParamsKey({ key: "blah", defaultValue }),
        { routingOptions: { route: `/` } },
      );

      expect(result.current.value).toEqual(defaultValue);
    });

    it("Returns out URL value if key exists in URL (always ignoring default value)", async () => {
      const key = "blah";
      const value = "cats";

      const { result } = await renderHookWithAuth(
        () => useSearchParamsKey({ key, defaultValue: "I don't matter" }),
        { routingOptions: { route: `/?${key}=${value}` } },
      );

      expect(result.current.value).toEqual(value);
    });

    it("Does not have methods change previous values if 'key' argument changes during re-renders", async () => {
      const readonlyKey = "readonlyKey";
      const mutableKey = "mutableKey";
      const initialReadonlyValue = "readonly";
      const initialMutableValue = "mutable";

      const { result, rerender, getLocationSnapshot } =
        await renderHookWithAuth(({ key }) => useSearchParamsKey({ key }), {
          routingOptions: {
            route: `/?${readonlyKey}=${initialReadonlyValue}&${mutableKey}=${initialMutableValue}`,
          },
          renderOptions: { initialProps: { key: readonlyKey } },
        });

      const swapValue = "dogs";
      await rerender({ key: mutableKey });
      act(() => result.current.setValue(swapValue));
      await waitFor(() => expect(result.current.value).toEqual(swapValue));

      const snapshot1 = getLocationSnapshot();
      expect(snapshot1.search.get(readonlyKey)).toEqual(initialReadonlyValue);
      expect(snapshot1.search.get(mutableKey)).toEqual(swapValue);

      act(() => result.current.deleteValue());
      await waitFor(() => expect(result.current.value).toEqual(""));

      const snapshot2 = getLocationSnapshot();
      expect(snapshot2.search.get(readonlyKey)).toEqual(initialReadonlyValue);
      expect(snapshot2.search.get(mutableKey)).toEqual(null);
    });
  });

  describe("setValue method", () => {
    it("Updates state and URL when called with a new value", async () => {
      const key = "blah";
      const initialValue = "cats";

      const { result, getLocationSnapshot } = await renderHookWithAuth(
        () => useSearchParamsKey({ key }),
        { routingOptions: { route: `/?${key}=${initialValue}` } },
      );

      const newValue = "dogs";
      act(() => result.current.setValue(newValue));
      await waitFor(() => expect(result.current.value).toEqual(newValue));

      const { search } = getLocationSnapshot();
      expect(search.get(key)).toEqual(newValue);
    });
  });

  describe("deleteValue method", () => {
    it("Clears value for the given key from the state and URL when called", async () => {
      const key = "blah";
      const initialValue = "cats";

      const { result, getLocationSnapshot } = await renderHookWithAuth(
        () => useSearchParamsKey({ key }),
        { routingOptions: { route: `/?${key}=${initialValue}` } },
      );

      act(() => result.current.deleteValue());
      await waitFor(() => expect(result.current.value).toEqual(""));

      const { search } = getLocationSnapshot();
      expect(search.get(key)).toEqual(null);
    });
  });

  describe("Override behavior", () => {
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
      act(() => result.current.setValue(newValue));
      await waitFor(() => expect(customParams.get(key)).toEqual(newValue));
    });
  });
});
