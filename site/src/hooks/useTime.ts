import { useEffect, useState } from "react";

/**
 * useTime allows a component to rerender over time without a corresponding state change.
 * An example could be a relative timestamp (eg. "in 5 minutes") that should count down as it
 * approaches.
 *
 * This hook should only be used in components that are very simple, and that will not
 * create a lot of unnecessary work for the reconciler. Given that this hook will result in
 * the entire subtree being rerendered on a frequent interval, it's important that the subtree
 * remains small.
 *
 * @param active Can optionally be set to false in circumstances where updating over time is
 * not necessary.
 */
export function useTime(active: boolean = true) {
  const [, setTick] = useState(0);

  useEffect(() => {
    if (!active) {
      return;
    }

    const interval = setInterval(() => {
      setTick((i) => i + 1);
    }, 1000);

    return () => {
      clearInterval(interval);
    };
  }, [active]);
}
