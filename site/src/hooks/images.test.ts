import { useImagePreloading, useThrottledImageLoader } from "./images";

/**
 * This is weird and clunky, and the mocking seems to have more edge cases than
 * the code I'm trying to test.
 *
 * HTMLImageElement doesn't go through fetch, but still it might be worth trying
 * to integrate this with MSW
 */
let mode: "alwaysSucceed" | "alwaysFail" = "alwaysSucceed";

class MockImage {
  #src = "";
  onload: (() => void) | undefined = undefined;
  onerror: (() => void) | undefined = undefined;

  get src() {
    return this.#src;
  }

  set src(newSrc: string) {
    this.#src = newSrc;
    this.#simulateHttpRequest(newSrc);
  }

  #simulateHttpRequest(src: string) {
    const promise = new Promise<void>((resolve, reject) => {
      if (src === "") {
        reject();
      }

      const settlePromise = mode === "alwaysSucceed" ? resolve : reject;
      setTimeout(settlePromise, 100);
    });

    // Need arrow functions because onload/onerror are allowed to mutate in the
    // original HTMLImageElement
    void promise.then(() => this.onload?.());
    void promise.catch(() => this.onerror?.());
  }
}

beforeAll(() => {
  jest.useFakeTimers();

  jest.spyOn(global, "Image").mockImplementation(() => {
    return new MockImage() as unknown as HTMLImageElement;
  });
});

beforeEach(() => {
  mode = "alwaysSucceed";
});

afterAll(() => {
  jest.useRealTimers();
  jest.clearAllMocks();
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
