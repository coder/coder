import { FormikContextType } from "formik/dist/types";
import { getFormHelpers, nameValidator, onChangeTrimmed } from "./formUtils";
import { mockApiError } from "testHelpers/entities";

interface TestType {
  untouchedGoodField: string;
  untouchedBadField: string;
  touchedGoodField: string;
  touchedBadField: string;
}

const mockHandleChange = jest.fn();

const form = {
  errors: {
    untouchedGoodField: undefined,
    untouchedBadField: "oops!",
    touchedGoodField: undefined,
    touchedBadField: "oops!",
  },
  touched: {
    untouchedGoodField: false,
    untouchedBadField: false,
    touchedGoodField: true,
    touchedBadField: true,
  },
  handleChange: mockHandleChange,
  handleBlur: jest.fn(),
  getFieldProps: (name: string) => {
    return {
      name,
      onBlur: jest.fn(),
      onChange: jest.fn(),
      value: "",
    };
  },
} as unknown as FormikContextType<TestType>;

const nameSchema = nameValidator("name");

describe("form util functions", () => {
  describe("getFormHelpers", () => {
    describe("without API errors", () => {
      const getFieldHelpers = getFormHelpers<TestType>(form);
      const untouchedGoodResult = getFieldHelpers("untouchedGoodField");
      const untouchedBadResult = getFieldHelpers("untouchedBadField");
      const touchedGoodResult = getFieldHelpers("touchedGoodField");
      const touchedBadResult = getFieldHelpers("touchedBadField");
      it("populates the 'field props'", () => {
        expect(untouchedGoodResult.name).toEqual("untouchedGoodField");
        expect(untouchedGoodResult.onBlur).toBeDefined();
        expect(untouchedGoodResult.onChange).toBeDefined();
        expect(untouchedGoodResult.value).toBeDefined();
      });
      it("sets the id to the name", () => {
        expect(untouchedGoodResult.id).toEqual("untouchedGoodField");
      });
      it("sets error to true if touched and invalid", () => {
        expect(untouchedGoodResult.error).toBeFalsy;
        expect(untouchedBadResult.error).toBeFalsy;
        expect(touchedGoodResult.error).toBeFalsy;
        expect(touchedBadResult.error).toBeTruthy;
      });
      it("sets helperText to the error message if touched and invalid", () => {
        expect(untouchedGoodResult.helperText).toBeUndefined;
        expect(untouchedBadResult.helperText).toBeUndefined;
        expect(touchedGoodResult.helperText).toBeUndefined;
        expect(touchedBadResult.helperText).toEqual("oops!");
      });
    });
    describe("with API errors", () => {
      it("shows an error if there is only an API error", () => {
        const getFieldHelpers = getFormHelpers<TestType>(
          form,
          mockApiError({
            validations: [
              {
                field: "touchedGoodField",
                detail: "API error!",
              },
            ],
          }),
        );
        const result = getFieldHelpers("touchedGoodField");
        expect(result.error).toBeTruthy();
        expect(result.helperText).toEqual("API error!");
      });
      it("shows an error if there is only a validation error", () => {
        const getFieldHelpers = getFormHelpers<TestType>(form, {});
        const result = getFieldHelpers("touchedBadField");
        expect(result.error).toBeTruthy();
        expect(result.helperText).toEqual("oops!");
      });
      it("shows the API error if both are present", () => {
        const getFieldHelpers = getFormHelpers<TestType>(
          form,
          mockApiError({
            validations: [
              {
                field: "touchedBadField",
                detail: "API error!",
              },
            ],
          }),
        );
        const result = getFieldHelpers("touchedBadField");
        expect(result.error).toBeTruthy();
        expect(result.helperText).toEqual("API error!");
      });
    });
  });

  describe("onChangeTrimmed", () => {
    it("calls handleChange with trimmed value", () => {
      const event = {
        target: { value: " hello " },
      } as React.ChangeEvent<HTMLInputElement>;
      onChangeTrimmed<TestType>(form)(event);
      expect(mockHandleChange).toHaveBeenCalledWith({
        target: { value: "hello" },
      });
    });
  });

  describe("nameValidator", () => {
    it("allows a 1-letter name", () => {
      const validate = () => nameSchema.validateSync("a");
      expect(validate).not.toThrow();
    });

    it("allows a 32-letter name", () => {
      const input = Array(32).fill("a").join("");
      const validate = () => nameSchema.validateSync(input);
      expect(validate).not.toThrow();
    });

    it("allows 'test-3' to be used as name", () => {
      const validate = () => nameSchema.validateSync("test-3");
      expect(validate).not.toThrow();
    });

    it("allows '3-test' to be used as a name", () => {
      const validate = () => nameSchema.validateSync("3-test");
      expect(validate).not.toThrow();
    });

    it("disallows a 33-letter name", () => {
      const input = Array(33).fill("a").join("");
      const validate = () => nameSchema.validateSync(input);
      expect(validate).toThrow();
    });

    it("disallows a space", () => {
      const validate = () => nameSchema.validateSync("test 3");
      expect(validate).toThrow();
    });
  });
});
