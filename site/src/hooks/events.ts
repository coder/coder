import { useEffect } from "react";
import { CustomEventListener } from "utils/events";

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
  useEffect(() => {
    const handleEvent: CustomEventListener<T> = (event) => {
      listener(event);
    };
    window.addEventListener(eventType, handleEvent as EventListener);

    return () => {
      window.removeEventListener(eventType, handleEvent as EventListener);
    };
  }, [eventType, listener]);
};
