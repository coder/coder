import { buildPagedList } from "./PaginationWidget"

describe("unit/PaginationWidget", () => {
  describe("buildPagedList", () => {
    it.each<{
      numPages: number
      activePage: number
      expected: (string | number)[]
    }>([
      { numPages: 7, activePage: 1, expected: [1, 2, 3, 4, 5, 6, 7] },
      { numPages: 17, activePage: 1, expected: [1, 2, 3, 4, 5, "right", 17] },
      {
        numPages: 17,
        activePage: 9,
        expected: [1, "left", 8, 9, 10, "right", 17],
      },
      {
        numPages: 17,
        activePage: 17,
        expected: [1, "left", 13, 14, 15, 16, 17],
      },
    ])(
      `buildPagedList($numPages, $activePage)`,
      ({ numPages, activePage, expected }) => {
        expect(buildPagedList(numPages, activePage)).toEqual(expected)
      },
    )
  })
})
