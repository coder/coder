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
        annotations: { format_duration: "false" },
      },
      expected: 3600000000000,
    },
    {
      option: {
        ...defaultOption,
        name: "Max Token Lifetime",
        value: 24 * 3600 * 1e9,
        annotations: { format_duration: "true" },
      },
      expected: "1 day",
    },
    {
      option: {
        ...defaultOption,
        name: "Session Duration",
        value: 3600 * 1e9,
        annotations: { format_duration: "some non-truthful value" },
      },
      expected: 3600000000000,
    },
    {
      option: {
        ...defaultOption,
        name: "Session Duration",
        value: 24 * 3600 * 1e9,
        annotations: { format_duration: "true" },
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
      expected: {
        single_tailnet: false,
        deployment_health_page: false,
        moons: true,
      },
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
    {
      option: {
        ...defaultOption,
        name: "Experiments",
        value: ["*", "moons", "single_tailnet"],
      },
      additionalValues: ["single_tailnet"],
      expected: { moons: true, single_tailnet: true },
    },
    {
      option: {
        ...defaultOption,
        name: "Some Go Duration We Want To Show As A String",
        value: 30 * 1e9,
        annotations: { format_duration: "true" },
      },
      expected: "30 seconds",
    },
    {
      option: {
        ...defaultOption,
        name: "Some Other Go Duration We Want To Just Display As A Number",
        value: 30 * 1e9,
        annotations: { format_duration: "false" },
      },
      expected: 30000000000,
    },
  ])(
    `[$option.name]optionValue($option.value)`,
    ({ option, expected, additionalValues }) => {
      expect(optionValue(option, additionalValues)).toEqual(expected);
    },
  );
});
