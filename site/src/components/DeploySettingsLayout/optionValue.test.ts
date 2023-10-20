import { optionValue } from "./optionValue";
import { ClibaseOption } from "api/typesGenerated";

const defaultOption: ClibaseOption = {
  name: "",
  description: "",
  flag: "",
  flag_shorthand: "",
  value: "",
  hidden: false,
};

describe("optionValue", () => {
  it.each<{
    option: ClibaseOption;
    additionalValues?: string[];
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
    {
      option: {
        ...defaultOption,
        name: "Experiments",
        value: ["single_tailnet"],
      },
      additionalValues: ["single_tailnet", "deployment_health_page"],
      expected: { single_tailnet: true, deployment_health_page: false },
    },
    {
      option: {
        ...defaultOption,
        name: "Experiments",
        value: [],
      },
      additionalValues: ["single_tailnet", "deployment_health_page"],
      expected: { single_tailnet: false, deployment_health_page: false },
    },
    {
      option: {
        ...defaultOption,
        name: "Experiments",
        value: ["moons"],
      },
      additionalValues: ["single_tailnet", "deployment_health_page"],
      expected: { single_tailnet: false, deployment_health_page: false },
    },
    {
      option: {
        ...defaultOption,
        name: "Experiments",
        value: ["*"],
      },
      additionalValues: ["single_tailnet", "deployment_health_page"],
      expected: { single_tailnet: true, deployment_health_page: true },
    },
  ])(
    `[$option.name]optionValue($option.value)`,
    ({ option, expected, additionalValues }) => {
      expect(optionValue(option, additionalValues)).toEqual(expected);
    },
  );
});
