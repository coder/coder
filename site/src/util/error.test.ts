import { errorString, Language } from "./error"

describe("error", () => {
  describe("errorStr", () => {
    it("returns message if error", () => {
      expect(errorString(new Error("foobar"))).toBe("foobar")
    })
    it("returns message if string", () => {
      expect(errorString("bazzle")).toBe("bazzle")
    })
    it("returns message if undefined or empty", () => {
      expect(errorString(undefined)).toBe(Language.noError)
      expect(errorString("")).toBe(Language.noError)
    })
    it("returns message if anything else", () => {
      expect(errorString({ qux: "fred" })).toBe(Language.unexpectedError)
      expect(errorString({ qux: 1 })).toBe(Language.unexpectedError)
    })
  })
})
