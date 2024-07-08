import { useEffect } from "react";
import type { CustomEventListener } from "utils/events";
import { useEffectEvent } from "./hookPolyfills";

/**
 * Handles a custom event with descriptive type information.
 *
 * @param eventType a unique name defining the type of the event. e.g. `"coder:workspace:ready"`
 * @param listener a custom event listener.
 */
export const useCustomEvent = <T, E extends string = string>(
  eventType: E,
  listener: CustomEventListener<T>,
): void => {
  // Ensures that the useEffect call only re-syncs when the eventType changes,
  // without needing parent component to memoize via useCallback
  const stableListener = useEffectEvent(listener);

  useEffect(() => {
    window.addEventListener(eventType, stableListener as EventListener);
    return () => {
      window.removeEventListener(eventType, stableListener as EventListener);
    };
  }, [stableListener, eventType]);
};
