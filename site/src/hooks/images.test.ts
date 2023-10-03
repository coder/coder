/**
 * Work in progress. Mainly because I need to figure out how best to deal with
 * a global constructor that implicitly makes HTTP requests in the background.
 */
import { useImagePreloading, useThrottledImageLoader } from "./images";

/**
 * Probably not on the right track with this one. Probably need to redo.
 */
class MockImage {
  #unusedWidth = 0;
  #unusedHeight = 0;
  #src = "";
  completed = false;

  constructor(width?: number, height?: number) {
    this.#unusedWidth = width ?? 0;
    this.#unusedHeight = height ?? 0;
  }

  get src() {
    return this.#src;
  }

  set src(newSrc: string) {
    this.#src = newSrc;
  }
}

beforeAll(() => {
  jest.useFakeTimers();
  jest.spyOn(global, "Image").mockImplementation(MockImage);
});

test(`${useImagePreloading.name}`, () => {
  it.skip("Should passively preload images after a render", () => {
    expect.hasAssertions();
  });

  it.skip("Should kick off a new pre-load if the content of the images changes after a re-render", () => {
    expect.hasAssertions();
  });

  it.skip("Should not kick off a new pre-load if the array changes by reference, but the content is the same", () => {
    expect.hasAssertions();
  });
});

test(`${useThrottledImageLoader.name}`, () => {
  it.skip("Should pre-load all images passed in the first time the function is called, no matter what", () => {
    expect.hasAssertions();
  });

  it.skip("Should throttle all calls to the function made within the specified throttle time", () => {
    expect.hasAssertions();
  });

  it.skip("Should always return a cleanup function", () => {
    expect.hasAssertions();
  });

  it.skip("Should reset its own state if the returned-out cleanup function is called", () => {
    expect.hasAssertions();
  });

  it.skip("Should not trigger the throttle if the images argument is undefined", () => {
    expect.hasAssertions();
  });

  it.skip("Should support arbitrary throttle values", () => {
    expect.hasAssertions();
  });

  it.skip("Should reset all of its state if the throttle value passed into the hook changes in a re-render", () => {
    expect.hasAssertions();
  });
});
