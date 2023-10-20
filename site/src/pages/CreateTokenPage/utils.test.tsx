import {
  filterByMaxTokenLifetime,
  determineDefaultLtValue,
  lifetimeDayPresets,
  LifetimeDay,
  NANO_HOUR,
} from "./utils";

describe("unit/CreateTokenForm", () => {
  describe("filterByMaxTokenLifetime", () => {
    it.each<{
      maxTokenLifetime: number;
      expected: LifetimeDay[];
    }>([
      { maxTokenLifetime: 6 * 24 * NANO_HOUR, expected: [] },
      {
        maxTokenLifetime: 20 * 24 * NANO_HOUR,
        expected: [lifetimeDayPresets[0]],
      },
      {
        maxTokenLifetime: 40 * 24 * NANO_HOUR,
        expected: [lifetimeDayPresets[0], lifetimeDayPresets[1]],
      },
      {
        maxTokenLifetime: 70 * 24 * NANO_HOUR,
        expected: [
          lifetimeDayPresets[0],
          lifetimeDayPresets[1],
          lifetimeDayPresets[2],
        ],
      },
      {
        maxTokenLifetime: 100 * 24 * NANO_HOUR,
        expected: lifetimeDayPresets,
      },
    ])(
      `filterByMaxTokenLifetime($maxTokenLifetime)`,
      ({ maxTokenLifetime, expected }) => {
        expect(filterByMaxTokenLifetime(maxTokenLifetime)).toEqual(expected);
      },
    );
  });
  describe("determineDefaultLtValue", () => {
    it.each<{
      maxTokenLifetime: number;
      expected: string | number;
    }>([
      {
        maxTokenLifetime: 0,
        expected: 30,
      },
      {
        maxTokenLifetime: 60 * 24 * NANO_HOUR,
        expected: 30,
      },
      {
        maxTokenLifetime: 20 * 24 * NANO_HOUR,
        expected: 7,
      },
      {
        maxTokenLifetime: 2 * 24 * NANO_HOUR,
        expected: "custom",
      },
    ])(
      `determineDefaultLtValue($maxTokenLifetime)`,
      ({ maxTokenLifetime, expected }) => {
        expect(determineDefaultLtValue(maxTokenLifetime)).toEqual(expected);
      },
    );
  });
});
