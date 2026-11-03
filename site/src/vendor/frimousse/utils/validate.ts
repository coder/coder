// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

// A suite of tiny validators.
// They are only meant to be used internally, so they don't throw human-friendly errors.

export type Validator<T> = (data: unknown) => T;

export function optional<T>(
  validate: Validator<T>,
): (data: unknown) => T | undefined {
  return (data: unknown): T | undefined =>
    data === undefined ? undefined : validate(data);
}

export function nullable<T>(validate: Validator<T>): Validator<T | null> {
  return (data: unknown): T | null => (data === null ? null : validate(data));
}

export function string(data: unknown): string {
  if (typeof data !== "string") {
    throw new Error();
  }

  return data;
}

export function number(data: unknown): number {
  if (typeof data !== "number") {
    throw new Error();
  }

  return data;
}

export function boolean(data: unknown): boolean {
  if (typeof data !== "boolean") {
    throw new Error();
  }

  return data;
}

export function object<T>(
  keys: {
    [K in keyof T]: Validator<T[K]>;
  },
): (data: unknown) => T {
  return (data: unknown): T => {
    if (typeof data !== "object" || data === null) {
      throw new Error();
    }

    const result = {} as T;

    for (const key in keys) {
      const value = (data as Record<keyof T, unknown>)[key];

      if (value === undefined) {
        // If the value is undefined, verify whether the validator allows it
        keys[key](undefined);
      }

      result[key as keyof T] = keys[key](value);
    }

    return result;
  };
}

export function naiveArray<T>(validate: Validator<T>): Validator<T[]> {
  return (data: unknown): T[] => {
    if (!Array.isArray(data)) {
      throw new Error();
    }

    // Naively only validate the first item
    if (data.length > 0) {
      validate(data[0]);
    }

    return data;
  };
}
