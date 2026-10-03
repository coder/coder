# Recorder

The recorder package defines a `Recorder` interface and supplementary types,
as well as several implementations of the interface. A concrete recorder,
like DRPCRecorder, is a Repository/Store for persisting information about
interactions with an LLM that were intercepted by AI gateway.

Various Decorator Recorders are defined that add behaviour like logging and tracing
without polluting the persistence layer. Decorator Recorders wrap other Recorders,
like middleware.

To begin exploring the package, start from types.go, which defines the Recorder
interface. Then, move on to the various implementations.

Decorators are composed with `Chain`, in chain.go, which takes them outermost
first and returns a function to apply them to the concrete recorder that
terminates the chain. Each decorating recorder has a corresponding `Decorator`
constructor, such as `WithLogging`, defined alongside it.
