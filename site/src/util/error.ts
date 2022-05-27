export const Language = {
  unexpectedError: "Unexpected error: see console for details",
  noError: "No error provided",
}

/**
 * Best effort to get a string from what could be an error or anything else.
 */
export const errorString = (error: Error | unknown): string | undefined => {
  if (error instanceof Error) {
    return error.message
  } else if (typeof error === "string") {
    return error || Language.noError
  } else if (typeof error !== "undefined") {
    console.warn(error)
    return Language.unexpectedError
  } else {
    return Language.noError
  }
}
