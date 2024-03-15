import { useEffect, useState } from "react";
import { useEffectEvent } from "./hookPolyfills";

interface UseTimeOptions {
  /**
   * Can be set to `true` to disable checking for updates in circumstances where it is known
   * that there is no work to do.
   */
  disabled?: boolean;

  /**
   * The amount of time in milliseconds that should pass between checking for updates.
   */
  interval?: number;
}

/**
 * useTime allows a component to rerender over time without a corresponding state change.
 * An example could be a relative timestamp (eg. "in 5 minutes") that should count down as it
 * approaches.
 */
export function useTime<T>(func: () => T, options: UseTimeOptions = {}): T {
  const [computedValue, setComputedValue] = useState(() => func());
  const { disabled = false, interval = 1000 } = options;

  const thunk = useEffectEvent(func);

  useEffect(() => {
    if (disabled) {
      return;
    }

    const handle = setInterval(() => {
      setComputedValue(() => thunk());
    }, interval);

    return () => {
      clearInterval(handle);
    };
  }, [thunk, disabled, interval]);

  return computedValue;
}
