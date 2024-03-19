import { buildPagedList, getOffset, isNonInitialPage } from "./utils";

describe(buildPagedList.name, () => {
  it("has no placeholder entries when there are seven or fewer pages", () => {
    for (let i = 1; i <= 7; i++) {
      const expectedResult: number[] = [];
      for (let j = 1; j <= i; j++) {
        expectedResult.push(j);
      }

      expect(buildPagedList(i, i)).toEqual(expectedResult);
    }
  });

  it("has 'right' placeholder for long lists when active page is near beginning", () => {
    expect(buildPagedList(17, 1)).toEqual([1, 2, 3, 4, 5, "right", 17]);
  });

  it("has 'left' placeholder for long lists when active page is near end", () => {
    expect(buildPagedList(17, 17)).toEqual([1, "left", 13, 14, 15, 16, 17]);
  });

  it("has both placeholders for long lists when active page is in the middle", () => {
    expect(buildPagedList(17, 9)).toEqual([1, "left", 8, 9, 10, "right", 17]);
  });

  it("produces an empty array when there are no pages", () => {
    expect(buildPagedList(0, 0)).toEqual([]);
  });

  it("makes sure all values are unique (for React rendering keys)", () => {
    type TestEntry = [numPages: number, activePage: number];
    const testData: TestEntry[] = [
      [0, 0],
      [1, 1],
      [2, 2],
      [3, 3],
      [4, 4],
      [5, 5],
      [6, 6],
      [7, 7],

      [10, 3],
      [7, 1],
      [17, 1],
      [17, 9],
    ];

    for (const [numPages, activePage] of testData) {
      const result = buildPagedList(numPages, activePage);
      const uniqueCount = new Set(result).size;

      expect(uniqueCount).toEqual(result.length);
    }
  });

  it("works for invalid active page number", () => {
    expect(buildPagedList(2, 4)).toEqual([1, 2]);
  });
});

describe(getOffset.name, () => {
  it("returns 0 on page 1", () => {
    const page = 1;
    const limit = 10;
    expect(getOffset(page, limit)).toEqual(0);
  });

  it("Returns the results for page 1 when input is invalid", () => {
    const inputs = [0, -1, -Infinity, NaN, Infinity, 3.6, 7.4545435];

    for (const input of inputs) {
      expect(getOffset(input, 10)).toEqual(0);
    }
  });

  it("Returns offset based on the current page for all valid pages after 1", () => {
    expect(getOffset(2, 10)).toEqual(10);
    expect(getOffset(3, 10)).toEqual(20);
    expect(getOffset(4, 45)).toEqual(135);
  });
});

describe(isNonInitialPage.name, () => {
  it("Should detect your page correctly if input is set and valid", () => {
    const params1 = new URLSearchParams({ page: String(1) });
    expect(isNonInitialPage(params1)).toBe(false);

    const inputs = [2, 50, 500, 3722];

    for (const input of inputs) {
      const params = new URLSearchParams({ page: String(input) });
      expect(isNonInitialPage(params)).toBe(true);
    }
  });

  it("Should act as if you are on page 1 if input is set but invalid", () => {
    const inputs = ["", Infinity, -Infinity, NaN, 3.74, -3];

    for (const input of inputs) {
      const params = new URLSearchParams({ page: String(input) });
      expect(isNonInitialPage(params)).toBe(false);
    }
  });

  it("Should act as if you are on page 1 if input does not exist", () => {
    const params = new URLSearchParams();
    expect(isNonInitialPage(params)).toBe(false);
  });
});
