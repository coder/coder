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

// firstParameter is mutable string with a default value (parameter value not required).
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

// secondParameter is immutable string with a default value (parameter value not required).
export const secondParameter: RichParameter = {
  ...emptyParameter,

  name: "second_parameter",
  displayName: "Second parameter",
  type: "string",
  options: [],
  description: "This is second parameter.",
  defaultValue: "abc",
  icon: "",
  order: 2,
}

// thirdParameter is mutable string with an empty default value (parameter value not required).
export const thirdParameter: RichParameter = {
  ...emptyParameter,

  name: "third_parameter",
  type: "string",
  options: [],
  description: "This is third parameter.",
  defaultValue: "",
  mutable: true,
  order: 3,
}

// fourthParameter is immutable boolean with a default "true" value (parameter value not required).
export const fourthParameter: RichParameter = {
  ...emptyParameter,

  name: "fourth_parameter",
  type: "bool",
  options: [],
  description: "This is fourth parameter.",
  defaultValue: "true",
  icon: "",
  order: 3,
}

// fifthParameter is immutable "string with options", with a default option selected (parameter value not required).
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
  order: 3,
}

// sixthParameter is mutable string without a default value (parameter value is required).
export const sixthParameter: RichParameter = {
  ...emptyParameter,

  name: "sixth_parameter",
  displayName: "Sixth parameter",
  type: "number",
  options: [],
  description: "This is sixth parameter.",
  icon: "/emojis/1f310.png",
  required: true,
  mutable: true,
  order: 1,
}

// seventhParameter is immutable string without a default value (parameter value is required).
export const seventhParameter: RichParameter = {
  ...emptyParameter,

  name: "seventh_parameter",
  displayName: "Seventh parameter",
  type: "string",
  options: [],
  description: "This is seventh parameter.",
  required: true,
  order: 1,
}
