package pubsub

// ChatCapacityChannel notifies waiters that agent capacity may be available.
// Delivery is at most once, so waiters also poll.
const ChatCapacityChannel = "chat:capacity"
