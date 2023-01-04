# APITypings

This main.go generates typescript types from the codersdk types in Go.

# Features

- Supports Go types
  - [x] Basics (string/int/etc)
  - [x] Maps
  - [x] Slices
  - [x] Enums
  - [x] Pointers
  - [ ] External Types (uses `any` atm)
    - Some custom external types are hardcoded in (eg: time.Time)

## Type overrides

```golang
type Foo struct {
	// Force the typescript type to be a number
	CreatedAt time.Duration `json:"created_at" typescript:"number"`
}
```

## Ignore Types

Do not generate ignored types.

```golang
// @typescript-ignore InternalType
type InternalType struct {
	// ...
}
```

# Future Ideas

- Use a yaml config for overriding certain types
