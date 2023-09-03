import { TemplateVersionParameter } from "api/typesGenerated"
import { getInitialRichParameterValues } from "./richParameters"

test("getInitialRichParameterValues return default value when default build parameter is not valid", () => {
  const templateParameters: TemplateVersionParameter[] = [
    {
      name: "cpu",
      display_name: "CPU",
      description: "The number of CPU cores",
      description_plaintext: "The number of CPU cores",
      type: "string",
      mutable: true,
      default_value: "2",
      icon: "/icon/memory.svg",
      options: [
        {
          name: "2 Cores",
          description: "",
          value: "2",
          icon: "",
        },
        {
          name: "4 Cores",
          description: "",
          value: "4",
          icon: "",
        },
        {
          name: "6 Cores",
          description: "",
          value: "6",
          icon: "",
        },
        {
          name: "8 Cores",
          description: "",
          value: "8",
          icon: "",
        },
      ],
      required: false,
      ephemeral: false,
    },
  ]

  const cpuParameter = templateParameters[0]
  const [cpuParameterInitialValue] = getInitialRichParameterValues(
    templateParameters,
    [{ name: cpuParameter.name, value: "100" }],
  )

  expect(cpuParameterInitialValue.value).toBe(cpuParameter.default_value)
})
