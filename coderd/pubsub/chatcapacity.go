package pubsub

// ChatCapacityChannel is the deployment-wide pubsub channel that
// receives one message whenever a chat frees a concurrent-agent
// capacity slot. Payloads carry no data; subscribers re-run their
// claim attempt. Delivery is at-most-once, so waiters pair this with
// a fallback poll.
const ChatCapacityChannel = "chat:capacity"
