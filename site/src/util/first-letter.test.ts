import { firstLetter } from "./first-letter"

describe("first-letter", () => {
  it.each<[string, string]>([
    ["", ""],
    ["User", "U"],
    ["test", "T"],
  ])(`firstLetter(%p) returns %p`, (input, expected) => {
    expect(firstLetter(input)).toBe(expected)
  })
})
