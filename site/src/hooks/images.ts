/**
 * @file Defines general-purpose hooks/utilities for working with images.
 *
 * Mainly for pre-loading images to minimize periods when a UI renders with data
 * but takes a few extra seconds for the images to load in.
 */
import { useCallback, useEffect, useRef, useState } from "react";

const MAX_RETRIES = 3;

/**
 * Tracks the loading status of an individual image.
 */
type ImageTrackerEntry = {
  image: HTMLImageElement;
  status: "loading" | "error" | "success";
  retries: number;
};

/**
 * Tracks all images pre-loaded by hooks/utilities defined in this file. This
 * is a single source of shared mutable state.
 */
const imageTracker = new Map<string, ImageTrackerEntry>();

/**
 * General pre-load utility used throughout the file. Returns a cleanup function
 * to help work with React.useEffect.
 *
 * Currently treated as an implementation detail, so it's not exported, but it
 * might make sense to break it out into a separate file down the line.
 */
function preloadImages(imageUrls?: readonly string[]): () => void {
  if (imageUrls === undefined) {
    // Just a noop
    return () => {};
  }

  const retryTimeoutIds: number[] = [];

  for (const imgUrl of imageUrls) {
    const prevEntry = imageTracker.get(imgUrl);

    if (prevEntry === undefined) {
      const dummyImage = new Image();
      dummyImage.src = imgUrl;

      const entry: ImageTrackerEntry = {
        image: dummyImage,
        status: "loading",
        retries: 0,
      };

      dummyImage.onload = () => {
        entry.status = "success";
      };

      dummyImage.onerror = () => {
        if (imgUrl !== "") {
          entry.status = "error";
        }
      };

      imageTracker.set(imgUrl, entry);
      continue;
    }

    const skipRetry =
      prevEntry.status === "loading" ||
      prevEntry.status === "success" ||
      prevEntry.retries === MAX_RETRIES;

    if (skipRetry) {
      continue;
    }

    prevEntry.image.src = "";
    const retryId = window.setTimeout(() => {
      prevEntry.image.src = imgUrl;
      prevEntry.retries++;
    }, 0);

    retryTimeoutIds.push(retryId);
  }

  return () => {
    for (const id of retryTimeoutIds) {
      window.clearTimeout(id);
    }
  };
}

/**
 * Exposes a throttled version of preloadImages. Useful for tying pre-loads to
 * things like mouse hovering.
 *
 * The throttling state is always associated with the component instance,
 * meaning that one component being throttled won't prevent other components
 * from making requests.
 */
export function useThrottledImageLoader(throttleTimeMs = 500) {
  /**
   * It's possible to reduce the amount of state by only having the cleanup ref;
   * tried it, but it made the code a lot harder to read and reason about
   */
  const throttledRef = useRef(false);
  const componentCleanupRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    componentCleanupRef.current?.();
  }, [throttleTimeMs]);

  return useCallback(
    (imgUrls?: readonly string[]) => {
      if (throttledRef.current || imgUrls === undefined) {
        // Noop
        return () => {};
      }

      throttledRef.current = true;
      const imagesCleanup = preloadImages(imgUrls);

      const timeoutId = window.setTimeout(() => {
        throttledRef.current = false;
      }, throttleTimeMs);

      const componentCleanup = () => {
        imagesCleanup();
        window.clearTimeout(timeoutId);

        throttledRef.current = false;
        componentCleanupRef.current = null;
      };

      componentCleanupRef.current = componentCleanup;
      return componentCleanup;
    },
    [throttleTimeMs],
  );
}

/**
 * Sets up passive image-preloading for a component.
 *
 * Has logic in place to minimize the risks of an array being passed in, even
 * if the array's memory reference changes every render.
 */
export function useImagePreloading(imgUrls?: readonly string[]) {
  // Doing weird, hacky nonsense to guarantee useEffect doesn't run too often,
  // even if consuming component doesn't stabilize value of imgUrls
  const [cachedUrls, setCachedUrls] = useState(imgUrls);

  // For performance reasons, this array comparison only goes one level deep
  const changedByValue =
    cachedUrls !== imgUrls &&
    (imgUrls?.length !== cachedUrls?.length ||
      !cachedUrls?.every((url, index) => url === imgUrls?.[index]));

  /**
   * If this state dispatch were called inside useEffect, that would mean that
   * the component and all of its children would render in full and get painted
   * to the screen, the effect would fire, and then the component and all of its
   * children would need to re-render and re-paint.
   *
   * Not a big deal for small components; possible concern when this hook is
   * designed to be used anywhere and could be used at the top of the app.
   *
   * Calling the state dispatch inline means that the React invalidates the
   * current component's output immediately. So the current render finishes,
   * React flushes the state changes, throws away the render result, and
   * immediately redoes the render with the new state. This happens before any
   * painting happens, and before React even tries to touch the children.
   *
   * Basically, this pattern is weird and ugly, but it guarantees that no matter
   * how complicated a component is, the cost of updating the state is always
   * limited to one component total, and never any of its descendants. And the
   * cost is limited to redoing an internal React render, not a render plus a
   * set of DOM updates.
   *
   * @see {@link https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes}
   */
  if (changedByValue) {
    setCachedUrls(imgUrls);
  }

  useEffect(() => {
    return preloadImages(cachedUrls);
  }, [cachedUrls]);
}
