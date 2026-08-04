package pubsub

// ChatCapacityChannel notifies waiters when concurrent-agent capacity may
// be available. Delivery is at-most-once, so waiters also use a fallback
// poll.
const ChatCapacityChannel = "chat:capacity"
