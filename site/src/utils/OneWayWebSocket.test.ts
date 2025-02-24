/**
 * Should probably bring in MSW to test the communication
 *
 * Qualities to test:
 * 1. When an event (open, close, error, message) triggers, any registered
 *    callbacks are called with a new event payload
 * 2. Lets user remove event listeners, as long as they provide a callback
 * 3. Is able to register multiple listeners for the same event type
 * 4. Lets user close a connection from the client side
 *
 * These aren't super high-priority right now, but if the definition ever
 * changes, it'd be good to have these as safety nets
 */
