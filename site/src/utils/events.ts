/**
 * Dispatches a custom event with descriptive type information.
 *
 * @param eventType a unique name defining the type of the event. e.g. `"coder:workspace:ready"`
 * @param detail an optional payload accessible to an event listener.
 * @param target an optional event target. Defaults to current `window`.
 */
export const dispatchCustomEvent = <D = unknown>(
  eventType: string,
  detail?: D,
  target: EventTarget = window,
): CustomEvent<D> => {
  const event = new CustomEvent<D>(eventType, { detail });

  target.dispatchEvent(event);

  return event;
};
/** Annotates a custom event listener with descriptive type information. */
export type CustomEventListener<D = unknown> = (event: CustomEvent<D>) => void;

/**
 * Determines if an Event object is a CustomEvent.
 *
 * @remark this is especially necessary when an event originates from an iframe
 * as `instanceof` will not match against another origin's prototype chain.
 */
export const isCustomEvent = <D = unknown>(
  event: CustomEvent<D> | Event,
): event is CustomEvent<D> => {
  return Boolean((event as CustomEvent).detail);
};
