import { RichParameter } from "./provisionerGenerated"

// Rich parameters

const emptyParameter: RichParameter = {
  name: "",
  description: "",
  type: "",
  mutable: false,
  defaultValue: "",
  icon: "",
  options: [],
  validationRegex: "",
  validationError: "",
  validationMin: undefined,
  validationMax: undefined,
  validationMonotonic: "",
  required: false,
  displayName: "",
  order: 0,
  ephemeral: false,
}

export const firstParameter: RichParameter = {
  ...emptyParameter,

  name: "first_parameter",
  displayName: "First parameter",
  type: "number",
  options: [],
  description: "This is first parameter.",
  icon: "/emojis/1f310.png",
  defaultValue: "123",
  mutable: true,
  order: 1,
}

export const secondParameter: RichParameter = {
  ...emptyParameter,

  name: "second_parameter",
  displayName: "Second parameter",
  type: "string",
  options: [],
  description: "This is second parameter.",
  defaultValue: "abc",
  icon: "",
  mutable: false,
  required: false,
  order: 2,
}

export const thirdParameter: RichParameter = {
  ...emptyParameter,

  name: "third_parameter",
  type: "string",
  options: [],
  description: "This is third parameter.",
  defaultValue: "",
  mutable: true,
  required: false,
  order: 3,
}

export const fourthParameter: RichParameter = {
  ...emptyParameter,

  name: "fourth_parameter",
  type: "bool",
  options: [],
  description: "This is fourth parameter.",
  defaultValue: "true",
  icon: "",
  mutable: false,
  required: true,
  order: 3,
}

export const fifthParameter: RichParameter = {
  ...emptyParameter,

  name: "fifth_parameter",
  displayName: "Fifth parameter",
  type: "string",
  options: [
    {
      name: "ABC",
      description: "This is ABC",
      value: "abc",
      icon: "",
    },
    {
      name: "DEF",
      description: "This is DEF",
      value: "def",
      icon: "",
    },
    {
      name: "GHI",
      description: "This is GHI",
      value: "ghi",
      icon: "",
    },
  ],
  description: "This is fifth parameter.",
  defaultValue: "def",
  icon: "",
  mutable: false,
  required: false,
  order: 3,
}
