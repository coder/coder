import { optionValue } from "./OptionsTable";
import { DeploymentOption } from "api/types";

const defaultOption: DeploymentOption = {
  name: "",
  description: "",
  flag: "",
  flag_shorthand: "",
  value: "",
  hidden: false,
};

describe("optionValue", () => {
  it.each<{
    option: DeploymentOption;
    expected: unknown;
  }>([
    {
      option: {
        ...defaultOption,
        name: "Max Token Lifetime",
        value: 3600 * 1e9,
      },
      expected: "1 hour",
    },
    {
      option: {
        ...defaultOption,
        name: "Max Token Lifetime",
        value: 24 * 3600 * 1e9,
      },
      expected: "1 day",
    },
    {
      option: {
        ...defaultOption,
        name: "Session Duration",
        value: 3600 * 1e9,
      },
      expected: "1 hour",
    },
    {
      option: {
        ...defaultOption,
        name: "Session Duration",
        value: 24 * 3600 * 1e9,
      },
      expected: "1 day",
    },
    {
      option: {
        ...defaultOption,
        name: "Strict-Transport-Security",
        value: 1000,
      },
      expected: "1000s",
    },
    {
      option: {
        ...defaultOption,
        name: "OIDC Group Mapping",
        value: {
          "123": "foo",
          "456": "bar",
          "789": "baz",
        },
      },
      expected: [`"123"->"foo"`, `"456"->"bar"`, `"789"->"baz"`],
    },
  ])(`[$option.name]optionValue($option.value)`, ({ option, expected }) => {
    expect(optionValue(option)).toEqual(expected);
  });
});
