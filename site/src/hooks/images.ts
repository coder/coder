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
  const throttledRef = useRef(false);
  const loadedCleanupRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    loadedCleanupRef.current?.();
  }, [throttleTimeMs]);

  return useCallback(
    (imgUrls?: readonly string[]) => {
      if (throttledRef.current || imgUrls === undefined) {
        // Noop
        return () => {};
      }

      throttledRef.current = true;
      const cleanup = preloadImages(imgUrls);
      loadedCleanupRef.current = cleanup;

      const timeoutId = window.setTimeout(() => {
        throttledRef.current = false;
      }, throttleTimeMs);

      return () => {
        cleanup();
        loadedCleanupRef.current = null;

        window.clearTimeout(timeoutId);
        throttledRef.current = false;
      };
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

  // Very uncommon pattern, but it's based on something from the official React
  // docs, and the comparison should have no perceivable effect on performance
  if (cachedUrls !== imgUrls) {
    const changedByValue =
      imgUrls?.length !== cachedUrls?.length ||
      !cachedUrls?.every((url, index) => url === imgUrls?.[index]);

    if (changedByValue) {
      setCachedUrls(imgUrls);
    }
  }

  useEffect(() => {
    return preloadImages(cachedUrls);
  }, [cachedUrls]);
}
